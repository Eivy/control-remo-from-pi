package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-yaml/yaml"
	rpio "github.com/stianeikeland/go-rpio"
)

var config Config

func main() {
	f, err := os.Open("./config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}
	err = yaml.Unmarshal(b, &config)
	rpio.Open()
	defer rpio.Close()
	fmt.Println(config.User.ID)
	for k, a := range config.Appliances {
		fmt.Println(k)
		appliance := a
		in := rpio.Pin(appliance.SwitchPin)
		in.Mode(rpio.Input)
		out := rpio.Pin(appliance.StatusPin)
		out.Mode(rpio.Output)
		ch := make(chan rpio.State)
		go func() {
			before := in.Read()
			for {
				select {
				default:
					tmp := in.Read()
					if before != tmp {
						if appliance.Trigger == TriggerSYNC {
							ch <- tmp
						} else if appliance.Trigger == TriggerTOGGLE {
							if before == rpio.Low && tmp == rpio.High {
								if out.Read() == rpio.Low {
									if appliance.StatusType == StatusTypeREV {
										ch <- rpio.Low
									} else {
										ch <- rpio.High
									}
								} else {
									if appliance.StatusType == StatusTypeREV {
										ch <- rpio.High
									} else {
										ch <- rpio.Low
									}
								}
							}
						}
					}
					before = tmp
					time.Sleep(time.Millisecond * 100)
				}
			}
		}()
		go func() {
			switch appliance.Type {
			case ApplianceTypeLIGHT, ApplianceTypeTV:
				for {
					select {
					case v := <-ch:
						fmt.Println(v)
						if v == rpio.High {
							sendToRemo(appliance.ID, "on")
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.Low)
							} else {
								out.Write(rpio.High)
							}
						} else {
							sendToRemo(appliance.ID, "off")
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.High)
							} else {
								out.Write(rpio.Low)
							}
						}
					}
				}
			case ApplianceTypeIR:
				for {
					select {
					case v := <-ch:
						fmt.Println(v)
						if v == rpio.High {
							sendSignal(appliance.ID, appliance.OnSignal)
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.Low)
							} else {
								out.Write(rpio.High)
							}
						} else {
							sendSignal(appliance.ID, appliance.OffSignal)
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.High)
							} else {
								out.Write(rpio.Low)
							}
						}
					}
				}
			}
		}()
	}
	for {
		time.Sleep(time.Millisecond * 200)
	}
}

func sendToRemo(id, button string) {
	value := url.Values{}
	value.Add("button", button)
	req, _ := http.NewRequest("POST", fmt.Sprintf("https://api.nature.global/1/appliances/%s/light", id), strings.NewReader(value.Encode()))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.User.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	b, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		log.Println(err)
	}
	fmt.Println(string(b))
}

func sendSignal(id, signal string) {
	req, _ := http.NewRequest("POST", fmt.Sprintf("https://api.nature.global/1/signals/%s/send", signal), nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", config.User.ID))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	b, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		log.Println(err)
	}
	fmt.Println(string(b))
}
