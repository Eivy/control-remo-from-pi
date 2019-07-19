package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/go-yaml/yaml"
	rpio "github.com/stianeikeland/go-rpio"
	"github.com/tenntenn/natureremo"
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
	remoClient := natureremo.NewClient(config.User.ID)
	ctx := context.Background()
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
							if appliance.Type == ApplianceTypeLIGHT {
								remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "on")
							} else if appliance.Type == ApplianceTypeTV {
								remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "on")
							}
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.Low)
							} else {
								out.Write(rpio.High)
							}
						} else {
							if appliance.Type == ApplianceTypeLIGHT {
								remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "off")
							} else if appliance.Type == ApplianceTypeTV {
								remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "off")
							}
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
							remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OnSignal})
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.Low)
							} else {
								out.Write(rpio.High)
							}
						} else {
							remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OffSignal})
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
