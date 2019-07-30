package main

import (
	"flag"
	"io/ioutil"
	"os"

	"time"

	"github.com/go-yaml/yaml"
)

// Config is configuration
type Config struct {
	User struct {
		ID string `yaml:"ID"`
	} `yaml:"User"`
	Appliances    map[string]Appliance `yaml:"Appliances"`
	CheckInterval time.Duration        `yaml:"CeckInterval"`
	Server        *Server              `yaml:"Server"`
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
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(b, &config)
	return
}
