package main

import "github.com/tenntenn/natureremo"

type ApplianceType string

const (
	ApplianceTypeLight = "LIGHT"
	ApplianceTypeTV    = "TV"
	ApplianceTypeIR    = "IR"
	ApplianceTypeLocal = "LOCAL"
)

// Appliance is Appliance
type Appliance struct {
	ID           string               `yaml:"ID"`
	Name         string               `yaml:"Name"`
	Type         ApplianceType        `yaml:"Type"`
	SwitchPin    int                  `yaml:"SwitchPin"`
	StatusPin    int                  `yaml:"StatusPin"`
	StatusType   StatusType           `yaml:"StatusType"`
	OnButton     string               `yaml:"OnButton"`
	OffButton    string               `yaml:"OffButton"`
	OnSignal     string               `yaml:"OnSignal"`
	OffSignal    string               `yaml:"OffSignal"`
	OnLocal      *natureremo.IRSignal `yaml:"OnLocal"`
	OffLocal     *natureremo.IRSignal `yaml:"OffLocal"`
	Trigger      Trigger              `yaml:"Trigger"`
	Timer        string               `yaml:"Timer"`
	ConditionPin int                  `yaml:"ConditionPin"`
	Buttons      map[string]string    `yaml:"Buttons"`
	IP           string               `yaml:"IP"`
}
