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
var timer = make(map[string]*time.Timer)

func main() {
	var err error
	config, err = ReadConfig()
	if err != nil {
		log.Fatal(err)
	}
	remoClient = natureremo.NewClient(config.User.ID)
	ctx := context.Background()
	rpio.Open()
	defer rpio.Close()
	for _, a := range config.Appliances {
		appliance := a
		in := rpio.Pin(appliance.SwitchPin)
		in.Mode(rpio.Input)
		out := rpio.Pin(appliance.StatusPin)
		out.Mode(rpio.Output)
		var condition rpio.Pin
		if appliance.ConditionPin != 0 {
			condition = rpio.Pin(appliance.ConditionPin)
			condition.Mode(rpio.Input)
		}
		ch := make(chan rpio.State)
		go func() {
			before := in.Read()
			for {
				select {
				default:
					tmp := in.Read()
					if before != tmp {
						ch <- tmp
						before = tmp
					}
					time.Sleep(time.Millisecond * 100)
				}
			}
		}()
		go func() {
			for {
				select {
				case v := <-ch:
					fmt.Println(appliance.Name, v)
					if condition != 0 && condition.Read() == rpio.Low {
						break
					}
					switch appliance.Trigger {
					case TriggerTOGGLE:
						if v == rpio.Low {
							continue
						}
						if out.Read() == rpio.Low {
							if appliance.StatusType == StatusTypeREV {
								v = rpio.Low
							} else {
								v = rpio.High
							}
						} else {
							if appliance.StatusType != StatusTypeREV {
								v = rpio.Low
							} else {
								v = rpio.High
							}
						}
					case TriggerTimer:
						if v == rpio.Low {
							continue
						}
						d, err := time.ParseDuration(appliance.Timer)
						if err != nil {
							log.Println(appliance, err)
						}
						if timer[appliance.ID] == nil {
							remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OnSignal})
							fmt.Println("TIMER", appliance.Name, "Start")
							timer[appliance.ID] = time.AfterFunc(d, func() {
								remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OffSignal})
								timer[appliance.ID] = nil
							})
						} else {
							fmt.Println("TIMER", appliance.Name, "Restart")
							timer[appliance.ID].Reset(d)
						}
						continue
					}
					if v == rpio.High {
						switch appliance.Type {
						case ApplianceTypeLIGHT:
							remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "on")
						case ApplianceTypeTV:
							remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "on")
						case ApplianceTypeIR:
							remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OnSignal})
						}
						if appliance.StatusType == StatusTypeREV {
							out.Write(rpio.Low)
						} else {
							out.Write(rpio.High)
						}
					} else {
						switch appliance.Type {
						case ApplianceTypeLIGHT:
							remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "off")
						case ApplianceTypeTV:
							remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, "off")
						case ApplianceTypeIR:
							remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OffSignal})
						}
						if appliance.StatusType == StatusTypeREV {
							out.Write(rpio.High)
						} else {
							out.Write(rpio.Low)
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
		http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", config.Server.Port), nil)
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
			fmt.Println("missing")
			return
		}
		if a.ConditionPin != 0 {
			condition := rpio.Pin(a.ConditionPin)
			condition.Mode(rpio.Input)
			if condition.Read() == rpio.Low {
				return
			}
		}
		t := a.Type
		switch t {
		case ApplianceTypeLIGHT:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case ApplianceTypeTV:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case ApplianceTypeIR:
			switch a.Trigger {
			case TriggerTimer:
				d, err := time.ParseDuration(a.Timer)
				if err != nil {
					log.Println(err, r)
				}
				if timer[id] == nil {
					remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OnSignal})
					fmt.Println("TIMER", a.Name, "Start")
					timer[id] = time.AfterFunc(d, func() {
						remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OffSignal})
						timer[id] = nil
					})
				} else {
					fmt.Println("TIMER", a.Name, "Restart")
					timer[id].Reset(d)
				}
			default:
				if button == "on" {
					remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OnSignal})
				} else {
					remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OffSignal})
				}
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
