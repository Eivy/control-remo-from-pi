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
	for k, a := range config.Appliances {
		ID := k
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
						fmt.Println(appliance.ID, v)
						if v == rpio.High {
							if appliance.Type == ApplianceTypeLIGHT {
								remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: ID}, "on")
							} else if appliance.Type == ApplianceTypeTV {
								remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: ID}, "on")
							}
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.Low)
							} else {
								out.Write(rpio.High)
							}
						} else {
							if appliance.Type == ApplianceTypeLIGHT {
								remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: ID}, "off")
							} else if appliance.Type == ApplianceTypeTV {
								remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: ID}, "off")
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
							err := remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OnSignal})
							if err != nil {
								log.Println(err)
							}
							if appliance.StatusType == StatusTypeREV {
								out.Write(rpio.Low)
							} else {
								out.Write(rpio.High)
							}
						} else {
							err := remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OffSignal})
							if err != nil {
								log.Println(err)
							}
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
	t := config.CheckInterval
	if t == 0 {
		t = 20
	}
	interval := time.Tick(time.Second * t)
	for {
		select {
		case <-interval:
			as, err := remoClient.ApplianceService.GetAll(ctx)
			if err != nil {
				log.Println(err)
			}
			for _, a := range as {
				switch a.Type {
				case natureremo.ApplianceTypeLight:
					ap := config.Appliances[a.ID]
					if a.Light.State.Power == "on" {
						if ap.StatusType == StatusTypeSTR {
							rpio.Pin(ap.StatusPin).Write(rpio.High)
						} else {
							rpio.Pin(ap.StatusPin).Write(rpio.Low)
						}
					} else {
						if ap.StatusType == StatusTypeSTR {
							rpio.Pin(ap.StatusPin).Write(rpio.Low)
						} else {
							rpio.Pin(ap.StatusPin).Write(rpio.High)
						}
					}
				}
			}
		}
	}
}
