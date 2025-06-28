package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/eivy/control-remo-from-pi/metrics"
	"github.com/eivy/control-remo-from-pi/mqtt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tenntenn/natureremo"
)

var config Config
var timer = make(map[string]*time.Timer)
var metricsCollector *metrics.Collector
var mqttClient *mqtt.Client
var lastKnownStates = make(map[string]*ApplianceStatus)

func main() {
	var err error
	config, err = ReadConfig()
	if err != nil {
		log.Fatal(err)
	}
	
	// Get Remo secret from environment variable
	remoSecret := os.Getenv("REMO_SECRET")
	if remoSecret == "" {
		log.Fatal("REMO_SECRET environment variable is required")
	}
	remoClient = natureremo.NewClient(remoSecret)
	
	// Initialize MQTT client if broker is configured
	mqttBroker := os.Getenv("MQTT_BROKER")
	if mqttBroker != "" {
		mqttPortStr := os.Getenv("MQTT_PORT")
		if mqttPortStr == "" {
			mqttPortStr = "1883"
		}
		mqttPort, err := strconv.Atoi(mqttPortStr)
		if err != nil {
			log.Fatalf("Invalid MQTT_PORT: %v", err)
		}
		
		mqttConfig := mqtt.Config{
			Broker:   mqttBroker,
			Port:     mqttPort,
			Username: os.Getenv("MQTT_USERNAME"),
			Password: os.Getenv("MQTT_PASSWORD"),
			ClientID: os.Getenv("MQTT_CLIENT_ID"),
		}
		
		if mqttConfig.ClientID == "" {
			mqttConfig.ClientID = "remo-controller"
		}
		
		mqttClient = mqtt.NewClient(mqttConfig)
		if err := mqttClient.Connect(); err != nil {
			log.Printf("Failed to connect to MQTT broker: %v", err)
			mqttClient = nil
		} else {
			log.Printf("MQTT client initialized successfully")
		}
	}
	
	// Initialize metrics collector
	metricsCollector = metrics.NewCollector(remoClient, 60*time.Second)
	prometheus.MustRegister(metricsCollector)
	
	ctx := context.Background()
	
	// Start MQTT command subscription if client is available
	if mqttClient != nil {
		if err := mqttClient.SubscribeCommands(ctx, &MQTTCommandHandler{}); err != nil {
			log.Printf("Failed to subscribe to MQTT commands: %v", err)
		}
		mqttClient.StartStatusPublisher(ctx)
		
		// Update MQTT connection metrics
		if metricsCollector != nil {
			go func() {
				for {
					time.Sleep(10 * time.Second)
					connected := mqttClient.IsConnected()
					metricsCollector.UpdateMQTTMetrics(0, 0, connected)
				}
			}()
		}
	}
	
	t := config.CheckInterval
	if t == 0 {
		t = 20
	}
	fmt.Printf("%#v\n", config.Server)
	if config.Server != nil {
		go statusCheck(&ctx, t)
		http.HandleFunc("/", remoControl)
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", config.Server.Port), nil)
	} else {
		statusCheck(&ctx, t)
	}
}



func remoControl(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	
	v := r.URL.Query()
	id := v.Get("id")
	button := v.Get("button")
	fmt.Printf("web requested: %s, %s\n", id, button)
	
	// Default to success, will be updated if error occurs
	statusCode := 200
	defer func() {
		if metricsCollector != nil {
			duration := time.Since(start).Seconds()
			metricsCollector.UpdateAPIMetrics("remoControl", statusCode, duration, nil)
		}
	}()
	if button != "" {
		ctx := context.Background()
		a, ok := config.Appliances[id]
		if !ok {
			fmt.Println("missing")
			statusCode = 404
			w.WriteHeader(404)
			return
		}
		
		// Handle timer-based appliances specially
		if a.Trigger == TriggerTimer {
			d, err := time.ParseDuration(*a.Timer)
			if err != nil {
				log.Printf("Invalid timer duration for appliance %s: %v", a.ID, err)
				statusCode = 400
				w.WriteHeader(400)
				return
			}
			
			if timer[a.ID] == nil {
				fmt.Println("TIMER", a.Name, "Start")
				// Execute ON command and publish status
				if err := executeApplianceCommandAndPublishStatus(ctx, a, "on"); err != nil {
					log.Printf("Failed to execute timer ON command: %v", err)
					statusCode = 500
					w.WriteHeader(500)
					return
				}
				
				// Set timer to turn off later
				timer[a.ID] = time.AfterFunc(d, func() {
					fmt.Println("TIMER", a.Name, "End")
					executeApplianceCommandAndPublishStatus(context.Background(), a, "off")
					timer[a.ID] = nil
				})
			} else {
				fmt.Println("TIMER", a.Name, "Restart")
				timer[a.ID].Reset(d)
			}
		} else {
			// Execute command and publish status based on actual API response
			if err := executeApplianceCommandAndPublishStatus(ctx, a, button); err != nil {
				log.Printf("Failed to execute command %s for appliance %s: %v", button, a.ID, err)
				statusCode = 500
				w.WriteHeader(500)
				return
			}
		}
	} else {
		a, ok := config.Appliances[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if a.Type != ApplianceTypeLight {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Status check removed as GPIO functionality was removed
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func statusCheck(ctx *context.Context, intervalSec time.Duration) {
	interval := time.Tick(time.Second * intervalSec)
	for {
		select {
		case <-interval:
			start := time.Now()
			as, err := remoClient.ApplianceService.GetAll(*ctx)
			duration := time.Since(start).Seconds()
			
			if err != nil {
				log.Println(err)
				// Record API error metrics
				if metricsCollector != nil {
					metricsCollector.UpdateAPIMetrics("GetAll", 500, duration, nil)
				}
			} else {
				// Record successful API call metrics
				if metricsCollector != nil {
					metricsCollector.UpdateAPIMetrics("GetAll", 200, duration, nil)
				}
			}
			
			for _, a := range as {
				ap := config.Appliances[a.ID]
				
				// Get appliance status
				status, err := getApplianceStatusFromAPIResponse(a)
				if err != nil {
					log.Printf("Failed to parse status for appliance %s: %v", a.ID, err)
					continue
				}
				
				// Update display for lights
				if a.Type == natureremo.ApplianceTypeLight && ap.display != nil {
					ap.display.Set(a.Light.State.Power)
					ap.display.Show()
				}
				
				// Update appliance metrics
				if metricsCollector != nil {
					metricsCollector.UpdateApplianceState(status.ID, status.Name, status.Type, status.PowerOn)
				}
				
				// Publish status change to MQTT only if changed
				publishApplianceStatusIfChanged(status.ID, status.Name, status.Type, status.PowerOn)
			}
		}
	}
}

// MQTTCommandHandler handles MQTT commands
type MQTTCommandHandler struct{}

// HandleCommand processes MQTT commands for appliance control
func (h *MQTTCommandHandler) HandleCommand(cmd mqtt.Command) error {
	ctx := context.Background()
	
	// Update MQTT received metrics
	if metricsCollector != nil {
		metricsCollector.IncrementMQTTReceived()
	}
	
	// Find the appliance by ID
	appliance, exists := config.Appliances[cmd.ApplianceID]
	if !exists {
		return fmt.Errorf("appliance not found: %s", cmd.ApplianceID)
	}
	
	// Execute the command and publish status based on actual API response
	return executeApplianceCommandAndPublishStatus(ctx, appliance, cmd.Button)
}

// ApplianceStatus represents the current status of an appliance
type ApplianceStatus struct {
	ID        string
	Name      string
	Type      string
	PowerOn   bool
	Available bool
}

// getApplianceStatus retrieves the current status of an appliance from Nature Remo API
func getApplianceStatus(ctx context.Context, applianceID string) (*ApplianceStatus, error) {
	appliances, err := remoClient.ApplianceService.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get appliances: %v", err)
	}
	
	for _, a := range appliances {
		if a.ID == applianceID {
			status := &ApplianceStatus{
				ID:        a.ID,
				Name:      a.Nickname,
				Available: true,
			}
			
			switch a.Type {
			case natureremo.ApplianceTypeLight:
				status.Type = "light"
				status.PowerOn = a.Light.State.Power == "on"
			case natureremo.ApplianceTypeTV:
				status.Type = "tv"
				// For TV, check if it has any available buttons (indicates it's responsive)
				status.PowerOn = len(a.TV.Buttons) > 0
			case natureremo.ApplianceTypeIR:
				status.Type = "ir"
				// For IR devices, assume they're available if they have signals
				status.PowerOn = len(a.Signals) > 0
			case natureremo.ApplianceTypeAirCon:
				status.Type = "aircon"
				// For AC, check if it's on based on operation mode
				if a.AirConSettings != nil {
					status.PowerOn = a.AirConSettings.OperationMode != ""
				} else {
					status.PowerOn = false
				}
			default:
				status.Type = "unknown"
				status.PowerOn = false
			}
			
			return status, nil
		}
	}
	
	return nil, fmt.Errorf("appliance not found: %s", applianceID)
}

// getApplianceStatusFromAPIResponse extracts status from Nature Remo API response
func getApplianceStatusFromAPIResponse(a *natureremo.Appliance) (*ApplianceStatus, error) {
	status := &ApplianceStatus{
		ID:        a.ID,
		Name:      a.Nickname,
		Available: true,
	}
	
	switch a.Type {
	case natureremo.ApplianceTypeLight:
		status.Type = "light"
		status.PowerOn = a.Light.State.Power == "on"
	case natureremo.ApplianceTypeTV:
		status.Type = "tv"
		// For TV, check if it has any available buttons (indicates it's responsive)
		status.PowerOn = len(a.TV.Buttons) > 0
	case natureremo.ApplianceTypeIR:
		status.Type = "ir"
		// For IR devices, assume they're available if they have signals
		status.PowerOn = len(a.Signals) > 0
	case natureremo.ApplianceTypeAirCon:
		status.Type = "aircon"
		// For AC, check if it's on based on operation mode
		if a.AirConSettings != nil {
			status.PowerOn = a.AirConSettings.OperationMode != ""
		} else {
			status.PowerOn = false
		}
	default:
		status.Type = "unknown"
		status.PowerOn = false
	}
	
	return status, nil
}

// publishApplianceStatusIfChanged publishes appliance status to MQTT only if changed
func publishApplianceStatusIfChanged(applianceID, applianceName, applianceType string, powerState bool) {
	// Check if the state has actually changed
	lastState, exists := lastKnownStates[applianceID]
	if exists && lastState.PowerOn == powerState {
		// State hasn't changed, don't publish
		return
	}
	
	// Update the last known state
	lastKnownStates[applianceID] = &ApplianceStatus{
		ID:        applianceID,
		Name:      applianceName,
		Type:      applianceType,
		PowerOn:   powerState,
		Available: true,
	}
	
	// Publish the status change
	publishApplianceStatusChange(applianceID, applianceName, applianceType, powerState)
}

// publishApplianceStatusChange publishes appliance status changes to MQTT
func publishApplianceStatusChange(applianceID, applianceName, applianceType string, powerState bool) {
	if mqttClient == nil {
		return
	}
	
	status := mqtt.Status{
		ApplianceID:   applianceID,
		ApplianceName: applianceName,
		Type:          applianceType,
		PowerState:    powerState,
		Timestamp:     time.Now(),
	}
	
	mqttClient.PublishStatusAsync(status)
	
	// Update MQTT metrics
	if metricsCollector != nil {
		metricsCollector.IncrementMQTTPublished()
	}
}

// executeApplianceCommandAndPublishStatus executes a command and publishes the resulting status
func executeApplianceCommandAndPublishStatus(ctx context.Context, appliance ApplianceData, command string) error {
	var err error
	
	// Execute the command
	switch command {
	case "on":
		err = executeApplianceOn(ctx, appliance)
	case "off":
		err = executeApplianceOff(ctx, appliance)
	case "toggle":
		err = executeApplianceToggle(ctx, appliance)
	default:
		// For other commands, just send the button
		appliance.sender.Send(ctx, command)
	}
	
	if err != nil {
		log.Printf("Failed to execute command %s for appliance %s: %v", command, appliance.ID, err)
		return err
	}
	
	// Wait a moment for the command to take effect
	time.Sleep(500 * time.Millisecond)
	
	// Get the current status and publish to MQTT
	status, err := getApplianceStatus(ctx, appliance.ID)
	if err != nil {
		log.Printf("Failed to get status for appliance %s after command: %v", appliance.ID, err)
		// Fallback: publish expected status based on command
		publishFallbackStatus(appliance, command)
		return nil
	}
	
	// Publish the actual status only if changed
	publishApplianceStatusIfChanged(status.ID, status.Name, status.Type, status.PowerOn)
	
	return nil
}

// executeApplianceOn turns on an appliance and returns any error
func executeApplianceOn(ctx context.Context, appliance ApplianceData) error {
	switch appliance.Type {
	case ApplianceTypeLight:
		appliance.sender.On(ctx)
	default:
		appliance.sender.Send(ctx, "on")
	}
	return nil
}

// executeApplianceOff turns off an appliance and returns any error
func executeApplianceOff(ctx context.Context, appliance ApplianceData) error {
	switch appliance.Type {
	case ApplianceTypeLight:
		appliance.sender.Off(ctx)
	default:
		appliance.sender.Send(ctx, "off")
	}
	return nil
}

// executeApplianceToggle toggles an appliance state
func executeApplianceToggle(ctx context.Context, appliance ApplianceData) error {
	if appliance.Type == ApplianceTypeLight {
		// For lights, get current status and toggle
		status, err := getApplianceStatus(ctx, appliance.ID)
		if err != nil {
			// Fallback: just send toggle command
			appliance.sender.Send(ctx, "toggle")
			return nil
		}
		
		if status.PowerOn {
			return executeApplianceOff(ctx, appliance)
		} else {
			return executeApplianceOn(ctx, appliance)
		}
	} else {
		// For other types, just send toggle command
		appliance.sender.Send(ctx, "toggle")
	}
	return nil
}

// publishFallbackStatus publishes expected status when API status check fails
func publishFallbackStatus(appliance ApplianceData, command string) {
	var powerState bool
	switch command {
	case "on":
		powerState = true
	case "off":
		powerState = false
	case "toggle":
		// For toggle, we can't know the state without checking, so skip publishing
		return
	default:
		// For other commands, assume device is on
		powerState = true
	}
	
	publishApplianceStatusChange(appliance.ID, appliance.Name, string(appliance.Type), powerState)
}



