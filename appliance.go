package main

// Appliance is Appliance
type Appliance struct {
	ID         string     `yaml:"ID"`
	Type       Type       `yaml:"Type"`
	SwitchPin  int        `yaml:"SwitchPin"`
	StatusPin  int        `yaml:"StatusPin"`
	StatusType StatusType `yaml:"StatusType"`
	OnSignal   string     `yaml:"OnSignal"`
	OffSignal  string     `yaml:"OffSignal"`
	Trigger    Trigger    `yaml:"Trigger"`
}
