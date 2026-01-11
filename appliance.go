package main

import (
	"context"

	"github.com/cormoran/natureremo"
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
	ID      string        `yaml:"ID"`
	Name    string        `yaml:"Name"`
	Type    ApplianceType `yaml:"Type"`
	Trigger Trigger       `yaml:"Trigger"`
	Timer   *string       `yaml:"Timer"`
	sender  Sender
	display Display
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
