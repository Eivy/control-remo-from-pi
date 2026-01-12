package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	pi "github.com/eivy/control-remo-from-pi"
	"github.com/eivy/control-remo-from-pi/mqtt"
	rpio "github.com/stianeikeland/go-rpio"
)

var config pi.Config
var mqttClient *mqtt.Client

func main() {
	var err error
	config, err = pi.ReadConfig()
	if err != nil {
		log.Fatal(err)
	}

	rpio.Open()
	ch := make(chan pi.ApplianceData)
	for _, a := range config.Appliances {
		fmt.Println(a.Name)
		in := rpio.Pin(*a.SwitchPin)
		in.Mode(rpio.Input)
		go pinCheck(in, a, ch)
		out := rpio.Pin(*a.StatusPin)
		out.Mode(rpio.Output)
	}

	// Initialize MQTT client if broker is configured
	mqttBroker := os.Getenv("MQTT_BROKER")
	if mqttBroker == "" {
		log.Fatal("set MQTT_BROKER")
	}
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
		log.Fatalf("Failed to connect to MQTT broker: %v", err)
	} else {
		log.Printf("MQTT client initialized successfully")
	}
	ctx := context.Background()

	// Start MQTT command subscription if client is available
	if err := mqttClient.SubscribeStatus(ctx, &MQTTStatusHandler{}); err != nil {
		log.Printf("Failed to subscribe to MQTT commands: %v", err)
	}
	buttonHandler(ctx, ch, mqttClient)
}

type MQTTStatusHandler struct{}

func (h *MQTTStatusHandler) HandleStatus(sts mqtt.Status) error {
	// Find the appliance by ID
	appliance, exists := config.Appliances[sts.ApplianceID]
	if !exists {
		return fmt.Errorf("appliance not found: %s", sts.ApplianceID)
	}
	out := rpio.Pin(*appliance.StatusPin)
	if sts.PowerState {
		out.Write(rpio.Low)
	} else {
		out.Write(rpio.High)
	}
	return nil
}

func pinCheck(in rpio.Pin, a pi.ApplianceData, ch chan pi.ApplianceData) {
	before := in.Read()
	for {
		tmp := in.Read()
		if before != tmp {
			ch <- a
		}
		before = tmp
		time.Sleep(time.Millisecond * 100)
	}
}

func buttonHandler(ctx context.Context, ch chan pi.ApplianceData, c *mqtt.Client) {
	for {
		select {
		case v := <-ch:
			fmt.Println(v.Name)
			switch v.Trigger {
			case pi.TriggerTOGGLE:
				c.PublishCommand(mqtt.Command{
					ApplianceID: v.ID,
					Button:      "toggle",
				})
			}
		}
	}
}
