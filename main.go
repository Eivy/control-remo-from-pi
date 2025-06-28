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
		a.sender.Send(ctx, button)
		t := a.Type
		switch t {
		case ApplianceTypeLight:
			a.sender.Send(ctx, button)
		case ApplianceTypeTV:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case ApplianceTypeIR, ApplianceTypeLocal:
			switch a.Trigger {
			case TriggerTimer:
				d, err := time.ParseDuration(*a.Timer)
				if err != nil {
					log.Println(err, r)
				}
				if timer[a.ID] == nil {
					a.sender.On(ctx)
					fmt.Println("TIMER", a.Name, "Start")
					timer[a.ID] = time.AfterFunc(d, func() {
						fmt.Println("TIMER", a.Name, "End")
						a.sender.Off(ctx)
						timer[a.ID] = nil
					})
				} else {
					fmt.Println("TIMER", a.Name, "Restart")
					timer[a.ID].Reset(d)
				}
			default:
				if button == "on" {
					a.sender.On(ctx)
					// Publish status change to MQTT
					if a.Type == ApplianceTypeLight {
						publishApplianceStatusChange(a.ID, a.Name, string(a.Type), true)
					}
				} else {
					a.sender.Off(ctx)
					// Publish status change to MQTT
					if a.Type == ApplianceTypeLight {
						publishApplianceStatusChange(a.ID, a.Name, string(a.Type), false)
					}
				}
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
				switch a.Type {
				case natureremo.ApplianceTypeLight:
					ap := config.Appliances[a.ID]
					ap.display.Set(a.Light.State.Power)
					ap.display.Show()
					
					powerState := a.Light.State.Power == "on"
					
					// Update appliance metrics
					if metricsCollector != nil {
						metricsCollector.UpdateApplianceState(a.ID, a.Nickname, "light", powerState)
					}
					
					// Publish status change to MQTT
					publishApplianceStatusChange(a.ID, a.Nickname, "light", powerState)
				}
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
	
	// Execute the command
	switch cmd.Button {
	case "on":
		appliance.sender.On(ctx)
	case "off":
		appliance.sender.Off(ctx)
	default:
		appliance.sender.Send(ctx, cmd.Button)
	}
	
	// Publish status change if this is a light appliance
	if appliance.Type == ApplianceTypeLight && mqttClient != nil {
		powerState := cmd.Button == "on" || (cmd.Button == "toggle" && appliance.display != nil)
		
		status := mqtt.Status{
			ApplianceID:   cmd.ApplianceID,
			ApplianceName: appliance.Name,
			Type:          string(appliance.Type),
			PowerState:    powerState,
			Timestamp:     time.Now(),
		}
		
		mqttClient.PublishStatusAsync(status)
	}
	
	return nil
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



