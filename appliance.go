package controlremo

import (
	"context"

	"github.com/cormoran/natureremo"
)

type ApplianceType string

const (
	ApplianceTypeLight = "LIGHT"
	ApplianceTypeTV    = "TV"
	ApplianceTypeIR    = "IR"
	ApplianceTypeLocal = "LOCAL"
)

// ApplianceData is ApplianceData
type ApplianceData struct {
	ID           string        `yaml:"ID"`
	Name         string        `yaml:"Name"`
	Type         ApplianceType `yaml:"Type"`
	SwitchPin    *int          `yaml:"SwitchPin"`
	StatusPin    *int          `yaml:"StatusPin"`
	Trigger      Trigger       `yaml:"Trigger"`
	Timer        *string       `yaml:"Timer"`
	ConditionPin *int          `yaml:"ConditionPin"`
	Sender       Sender
	Display      Display
}

type Sender interface {
	On(ctx context.Context) (*natureremo.LightState, error)
	Off(ctx context.Context) (*natureremo.LightState, error)
	Send(ctx context.Context, button string) (*natureremo.LightState, error)
}

type Display interface {
	Show()
	Get() error
	Set(string)
}
