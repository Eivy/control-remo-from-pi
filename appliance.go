package main

import (
	"context"

	"github.com/tenntenn/natureremo"
)

var remoClient *natureremo.Client

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
	StatusType   *StatusType   `yaml:"StatusType"`
	Trigger      Trigger       `yaml:"Trigger"`
	Timer        *string       `yaml:"Timer"`
	ConditionPin *int          `yaml:"ConditionPin"`
	sender       Sender
	display      Display
}

type Sender interface {
	On(ctx context.Context)
	Off(ctx context.Context)
	Send(ctx context.Context, button string)
}

type Display interface {
	Show()
	Get() error
	Set(string)
}
