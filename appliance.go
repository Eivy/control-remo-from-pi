package main

import "github.com/tenntenn/natureremo"

// Appliance is Appliance
type Appliance struct {
	ID           string                   `yaml:"ID"`
	Name         string                   `yaml:"Name"`
	Type         natureremo.ApplianceType `yaml:"Type"`
	SwitchPin    int                      `yaml:"SwitchPin"`
	StatusPin    int                      `yaml:"StatusPin"`
	StatusType   StatusType               `yaml:"StatusType"`
	OnButton     string                   `yaml:"OnButton"`
	OffButton    string                   `yaml:"OffButton"`
	OnSignal     string                   `yaml:"OnSignal"`
	OffSignal    string                   `yaml:"OffSignal"`
	Trigger      Trigger                  `yaml:"Trigger"`
	Timer        string                   `yaml:"Timer"`
	ConditionPin int                      `yaml:"ConditionPin"`
	Buttons      map[string]string        `yaml:"Buttons"`
}
