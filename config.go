package main

import "time"

// Config is configuration
type Config struct {
	User struct {
		ID string `yaml:"ID"`
	} `yaml:"User"`
	Appliances map[string]Appliance `yaml:"Appliances"`
	CheckInterval time.Duration
}
