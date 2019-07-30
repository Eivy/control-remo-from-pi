package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	rpio "github.com/stianeikeland/go-rpio"
	"github.com/tenntenn/natureremo"
)

var config Config
var remoClient *natureremo.Client

func main() {
	config, err := ReadConfig()
	if err != nil {
		log.Fatal(err)
	}
	remoClient = natureremo.NewClient(config.User.ID)
	ctx := context.Background()
	rpio.Open()
	defer rpio.Close()
	for k, a := range config.Appliances {
		name := k
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
						fmt.Println(name, v)
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
						fmt.Println(name, v)
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
	fmt.Printf("%#v\n", config.Server)
	if config.Server != nil {
		go statusCheck(&ctx, t)
		http.HandleFunc("/", remoControl)
		http.ListenAndServe(fmt.Sprintf(":%s", config.Server.Port), nil)
	} else {
		statusCheck(&ctx, t)
	}
}

func remoControl(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	id := v.Get("id")
	button := v.Get("button")
	if button != "" {
		ctx := context.Background()
		a, ok := config.Appliances[id]
		if !ok {
			return
		}
		t := a.Type
		switch t {
		case ApplianceTypeLIGHT:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case ApplianceTypeTV:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case ApplianceTypeIR:
			if button == "on" {
				remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OnSignal})
			} else {
				remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OffSignal})
			}
		}
	} else {
		pin := rpio.Pin(config.Appliances[id].StatusPin)
		s := pin.Read()
		if s == rpio.High {
			if config.Appliances[id].StatusType == StatusTypeSTR {
				fmt.Fprint(w, 1)
			} else {
				fmt.Fprint(w, 0)
			}
		} else {
			if config.Appliances[id].StatusType == StatusTypeSTR {
				fmt.Fprint(w, 0)
			} else {
				fmt.Fprint(w, 1)
			}
		}
	}
	return
}

func statusCheck(ctx *context.Context, intervalSec time.Duration) {
	interval := time.Tick(time.Second * intervalSec)
	for {
		select {
		case <-interval:
			as, err := remoClient.ApplianceService.GetAll(*ctx)
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
