package controlremo

import (
	"flag"
	"fmt"
	"io"
	"os"

	"time"

	"github.com/cormoran/natureremo"
	"gopkg.in/yaml.v3"
)

// Config is configuration
type Config struct {
	Appliances    map[string]ApplianceData `yaml:"Appliances"`
	CheckInterval time.Duration            `yaml:"CeckInterval"`
	Server        *Server                  `yaml:"Server"`
}

// ReadConfig returns config read from config file which is in excute path or specified in command args
func ReadConfig() (config Config, err error) {
	var configFile string
	flag.StringVar(&configFile, "config", "./config.yaml", "Config file to read")
	flag.Parse()
	f, err := os.Open(configFile)
	if err != nil {
		return
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return
	}
	var tmp struct {
		Appliances map[string]struct {
			ID           string              `yaml:"ID"`
			Name         string              `yaml:"Name"`
			Type         ApplianceType       `yaml:"Type"`
			SwitchPin    *int                `yaml:"SwitchPin"`
			StatusPin    *int                `yaml:"StatusPin"`
			Trigger      Trigger             `yaml:"Trigger"`
			Timer        *string             `yaml:"Timer"`
			ConditionPin *int                `yaml:"ConditionPin"`
			OnButton     *string             `yaml:"OnButton"`
			OffButton    *string             `yaml:"OffButton"`
			Status       *bool               // true is power on
			IP           string              `yaml:"IP"`
			OnLocal      natureremo.IRSignal `yaml:"OnLocal"`
			OffLocal     natureremo.IRSignal `yaml:"OffLocal"`
			OnSignal     string              `yaml:"OnSignal"`
			OffSignal    string              `yaml:"OffSignal"`
		} `yaml:"Appliances"`
		CheckInterval time.Duration `yaml:"CeckInterval"`
		Server        *Server       `yaml:"Server"`
	}
	err = yaml.Unmarshal(b, &tmp)
	appliances := make(map[string]ApplianceData)
	fmt.Println("reading config", len(tmp.Appliances))
	for k, v := range tmp.Appliances {
		tmp := ApplianceData{
			ID:           v.ID,
			Name:         v.Name,
			Type:         v.Type,
			SwitchPin:    v.SwitchPin,
			StatusPin:    v.StatusPin,
			Trigger:      v.Trigger,
			Timer:        v.Timer,
			ConditionPin: v.ConditionPin,
		}
		switch v.Type {
		case ApplianceTypeIR:
			tmp.Sender = ApplianceIR{
				ApplianceData: tmp,
				OnSignal:      v.OnSignal,
				OffSignal:     v.OffSignal,
			}
		case ApplianceTypeLight:
			l := ApplianceLight{
				ApplianceData: tmp,
				OnButton:      v.OnButton,
				OffButton:     v.OffButton,
				Status:        v.Status,
			}
			tmp.Sender = l
			tmp.Display = &l
		case ApplianceTypeLocal:
			tmp.Sender = ApplianceLocal{
				ApplianceData: tmp,
				IP:            v.IP,
				OnLocal:       v.OnLocal,
				OffLocal:      v.OffLocal,
			}
		case ApplianceTypeTV:
			tmp.Sender = ApplianceTV{
				ApplianceData: tmp,
				OnButton:      v.OnButton,
				OffButton:     v.OffButton,
			}
		}
		appliances[k] = tmp
	}
	config = Config{
		Server:        tmp.Server,
		CheckInterval: tmp.CheckInterval,
		Appliances:    appliances,
	}
	return
}
