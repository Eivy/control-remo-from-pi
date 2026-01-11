package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Config holds MQTT configuration
type Config struct {
	Broker   string
	Port     int
	Username string
	Password string
	ClientID string
}

// Client wraps MQTT client functionality
type Client struct {
	client      mqtt.Client
	config      Config
	commandChan chan Command
	statusChan  chan Status
}

// Command represents a remote control command
type Command struct {
	ApplianceID string `json:"appliance_id"`
	Button      string `json:"button"`
	Type        string `json:"type"` // "light", "tv", "ir", "local"
}

// Status represents appliance status change
type Status struct {
	ApplianceID   string    `json:"appliance_id"`
	ApplianceName string    `json:"appliance_name"`
	Type          string    `json:"type"`
	PowerState    bool      `json:"power_state"`
	Timestamp     time.Time `json:"timestamp"`
}

// CommandHandler defines the interface for handling MQTT commands
type CommandHandler interface {
	HandleCommand(cmd Command) error
}

// StatusHandler defines the interface for handling MQTT commands
type StatusHandler interface {
	HandleStatus(cmd Status) error
}

// NewClient creates a new MQTT client
func NewClient(config Config) *Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Broker, config.Port))
	opts.SetClientID(config.ClientID)
	opts.SetUsername(config.Username)
	opts.SetPassword(config.Password)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.SetPingTimeout(60 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(10 * time.Second)

	// Connection lost handler
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	})

	// On connect handler
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Println("MQTT connected successfully")
	})

	client := mqtt.NewClient(opts)

	return &Client{
		client:      client,
		config:      config,
		commandChan: make(chan Command, 100),
		statusChan:  make(chan Status, 100),
	}
}

// Connect establishes connection to MQTT broker
func (c *Client) Connect() error {
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %v", token.Error())
	}
	log.Printf("Connected to MQTT broker at %s:%d", c.config.Broker, c.config.Port)
	return nil
}

// Disconnect closes the connection to MQTT broker
func (c *Client) Disconnect() {
	c.client.Disconnect(250)
	close(c.commandChan)
	close(c.statusChan)
}

// SubscribeCommands subscribes to command topics and starts processing
func (c *Client) SubscribeCommands(ctx context.Context, handler CommandHandler) error {
	// Subscribe to command topic: remo/command/{appliance_id}
	commandTopic := "remo/command/+"

	token := c.client.Subscribe(commandTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		// Extract appliance ID from topic
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) != 3 {
			log.Printf("Invalid command topic format: %s", msg.Topic())
			return
		}
		applianceID := parts[2]

		// Parse command payload
		var payload struct {
			Button string `json:"button"`
			Type   string `json:"type,omitempty"`
		}

		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			log.Printf("Failed to parse command payload: %v", err)
			return
		}

		command := Command{
			ApplianceID: applianceID,
			Button:      payload.Button,
			Type:        payload.Type,
		}

		// Send to command channel for processing
		select {
		case c.commandChan <- command:
		default:
			log.Printf("Command channel full, dropping command for %s", applianceID)
		}
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to commands: %v", token.Error())
	}

	log.Printf("Subscribed to MQTT command topic: %s", commandTopic)

	// Start command processor
	go c.processCommands(ctx, handler)

	return nil
}

// SubscribeStatus subscribes to status topics and starts processing
func (c *Client) SubscribeStatus(ctx context.Context, handler StatusHandler) error {
	// Subscribe to command topic: remo/command/{appliance_id}
	statusTopic := "remo/status/+"

	token := c.client.Subscribe(statusTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		// Extract appliance ID from topic
		parts := strings.Split(msg.Topic(), "/")
		if len(parts) != 3 {
			log.Printf("Invalid command topic format: %s", msg.Topic())
			return
		}
		applianceID := parts[2]

		// Parse command payload
		var payload struct {
			PowerState bool   `json:"power_state"`
			Type       string `json:"type,omitempty"`
		}

		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			log.Printf("Failed to parse command payload: %v", err)
			return
		}

		status := Status{
			ApplianceID: applianceID,
			PowerState:  payload.PowerState,
			Type:        payload.Type,
		}

		// Send to command channel for processing
		select {
		case c.statusChan <- status:
		default:
			log.Printf("Command channel full, dropping command for %s", applianceID)
		}
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to commands: %v", token.Error())
	}

	log.Printf("Subscribed to MQTT command topic: %s", statusTopic)

	// Start command processor
	go c.processStatus(ctx, handler)

	return nil
}

// processCommands handles incoming commands from MQTT
func (c *Client) processCommands(ctx context.Context, handler CommandHandler) {
	for {
		select {
		case cmd := <-c.commandChan:
			if err := handler.HandleCommand(cmd); err != nil {
				log.Printf("Failed to handle command for %s: %v", cmd.ApplianceID, err)
			} else {
				log.Printf("Successfully handled command: %s -> %s", cmd.ApplianceID, cmd.Button)
			}
		case <-ctx.Done():
			return
		}
	}
}

// processStatus handles incoming status from MQTT
func (c *Client) processStatus(ctx context.Context, handler StatusHandler) {
	for {
		select {
		case cmd := <-c.statusChan:
			if err := handler.HandleStatus(cmd); err != nil {
				log.Printf("Failed to handle status for %s: %v", cmd.ApplianceID, err)
			} else {
				log.Printf("Successfully handled status: %s -> %v", cmd.ApplianceID, cmd.PowerState)
			}
		case <-ctx.Done():
			return
		}
	}
}

// PublishCommand publishes command
func (c *Client) PublishCommand(cmd Command) error {
	topic := fmt.Sprintf("remo/command/%s", cmd.ApplianceID)

	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %v", err)
	}

	token := c.client.Publish(topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish status: %v", token.Error())
	}

	log.Printf("Published command for %s: button=%t", cmd.ApplianceID, cmd.Button)
	return nil
}

// PublishStatus publishes appliance status changes
func (c *Client) PublishStatus(status Status) error {
	topic := fmt.Sprintf("remo/status/%s", status.ApplianceID)

	payload, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %v", err)
	}

	token := c.client.Publish(topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish status: %v", token.Error())
	}

	log.Printf("Published status for %s: power=%t", status.ApplianceName, status.PowerState)
	return nil
}

// PublishStatusAsync publishes status changes asynchronously
func (c *Client) PublishStatusAsync(status Status) {
	select {
	case c.statusChan <- status:
	default:
		log.Printf("Status channel full, dropping status for %s", status.ApplianceID)
	}
}

// StartStatusPublisher starts the background status publisher
func (c *Client) StartStatusPublisher(ctx context.Context) {
	go func() {
		for {
			select {
			case status := <-c.statusChan:
				if err := c.PublishStatus(status); err != nil {
					log.Printf("Failed to publish status: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// IsConnected checks if the client is connected
func (c *Client) IsConnected() bool {
	return c.client.IsConnected()
}

// GetConfig returns the current MQTT configuration
func (c *Client) GetConfig() Config {
	return c.config
}

// Default message handler
var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message: %s from topic: %s", msg.Payload(), msg.Topic())
}
