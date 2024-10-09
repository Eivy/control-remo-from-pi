package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
	rpio "github.com/stianeikeland/go-rpio"
	"github.com/tenntenn/natureremo"
)

var config Config
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
		fmt.Println(a.Name)
		ch := make(chan rpio.State)
		fmt.Println(*a.SwitchPin)
		in := rpio.Pin(*a.SwitchPin)
		in.Mode(rpio.Input)
		go pinCheck(*a.SwitchPin, in, ch)
		out := rpio.Pin(*a.StatusPin)
		out.Mode(rpio.Output)
		var condition rpio.Pin
		if a.ConditionPin != nil {
			condition = rpio.Pin(*a.ConditionPin)
			condition.Mode(rpio.Input)
		}
		if config.Host == nil {
			go serverSide(ctx, condition, out, ch, a)
		} else {
			go clientSide(ctx, out, ch, a)
		}
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

func getStatusFromHost(dist, id string) string {
	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s/?id=%s", dist, id))
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func sendButtonToHost(dist, id, button string) (err error) {
	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s/?id=%s&button=%s", dist, id, button))
	if err != nil {
		fmt.Println(err)
		return
	}
	res.Body.Close()
	return
}

func remoControl(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	id := v.Get("id")
	button := v.Get("button")
	fmt.Printf("web requested: %s, %s\n", id, button)
	if button != "" {
		ctx := context.Background()
		a, ok := config.Appliances[id]
		if !ok {
			fmt.Println("missing")
			return
		}
		if a.ConditionPin != nil {
			condition := rpio.Pin(*a.ConditionPin)
			condition.Mode(rpio.Input)
			if condition.Read() == rpio.Low {
				return
			}
		}
		a.sender.Send(ctx, button)
		t := a.Type
		switch t {
		case ApplianceTypeLight:
			if button == "toggle" {
				button = "on"
				if *a.StatusType == StatusTypeSTR {
					if rpio.Pin(*a.StatusPin).Read() == rpio.High {
						a.sender.Off(ctx)
						button = "off"
					} else {
						a.sender.On(ctx)
					}
				} else {
					if rpio.Pin(*a.StatusPin).Read() == rpio.Low {
						a.sender.Off(ctx)
						button = "off"
					} else {
						a.sender.On(ctx)
					}
				}
			} else {
				a.sender.Send(ctx, button)
			}
			if button == "on" {
				if *a.StatusType == StatusTypeSTR {
					rpio.Pin(*a.StatusPin).Write(rpio.High)
				} else {
					rpio.Pin(*a.StatusPin).Write(rpio.Low)
				}
			} else {
				if *a.StatusType == StatusTypeSTR {
					rpio.Pin(*a.StatusPin).Write(rpio.Low)
				} else {
					rpio.Pin(*a.StatusPin).Write(rpio.High)
				}
			}
		case ApplianceTypeTV:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case ApplianceTypeIR, ApplianceTypeLocal:
			switch a.Trigger {
			case TriggerTimer:
				d, err := time.ParseDuration(*a.Timer)
				if err != nil {
					log.Println(err, r)
				}
				if timer[a.ID] == nil {
					a.sender.On(ctx)
					fmt.Println("TIMER", a.Name, "Start")
					timer[a.ID] = time.AfterFunc(d, func() {
						fmt.Println("TIMER", a.Name, "End")
						a.sender.Off(ctx)
						timer[a.ID] = nil
					})
				} else {
					fmt.Println("TIMER", a.Name, "Restart")
					timer[a.ID].Reset(d)
				}
			default:
				if button == "on" {
					a.sender.On(ctx)
					rpio.Pin(*a.StatusPin).Write(rpio.High)
				} else {
					a.sender.Off(ctx)
					rpio.Pin(*a.StatusPin).Write(rpio.Low)
				}
			}
		}
	} else {
		a, ok := config.Appliances[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if a.Type != ApplianceTypeLight {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, *config.Appliances[id].StatusPin)
	}
}

func statusCheck(ctx *context.Context, intervalSec time.Duration) {
	if config.Host == nil {
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
						ap.display.Set(a.Light.State.Power)
						ap.display.Show()
					}
				}
			}
		}
	} else {
		for {
			interval := time.Tick(time.Second * 5)
			select {
			case <-interval:
				for _, a := range config.Appliances {
					if a.display == nil {
						continue
					}
					a.display.Show()
				}
			}
		}
	}
}

func pinCheck(num int, in rpio.Pin, ch chan rpio.State) {
	before := in.Read()
	for {
		select {
		default:
			tmp := in.Read()
			if before != tmp {
				fmt.Println(num)
				ch <- tmp
				before = tmp
			}
			time.Sleep(time.Millisecond * 100)
		}
	}
}

func serverSide(ctx context.Context, condition rpio.Pin, out rpio.Pin, ch chan rpio.State, appliance ApplianceData) {
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
					if *appliance.StatusType == StatusTypeREV {
						v = rpio.Low
					} else {
						v = rpio.High
					}
				} else {
					if *appliance.StatusType != StatusTypeREV {
						v = rpio.Low
					} else {
						v = rpio.High
					}
				}
			case TriggerTimer:
				if v == rpio.Low {
					continue
				}
				d, err := time.ParseDuration(*appliance.Timer)
				if err != nil {
					log.Println(appliance, err)
				}
				if timer[appliance.ID] == nil {
					fmt.Println("TIMER", appliance.Name, "Start")
					appliance.sender.On(ctx)
					timer[appliance.ID] = time.AfterFunc(d, func() {
						fmt.Println("TIMER", appliance.Name, "End")
						appliance.sender.Off(ctx)
						timer[appliance.ID] = nil
					})
					continue
				} else {
					fmt.Println("TIMER", appliance.Name, "Restart")
					timer[appliance.ID].Reset(d)
					continue
				}
			}
			if v == rpio.High {
				appliance.sender.On(ctx)
				if *appliance.StatusType == StatusTypeREV {
					out.Write(rpio.Low)
				} else {
					out.Write(rpio.High)
				}
			} else {
				appliance.sender.Off(ctx)
				if *appliance.StatusType == StatusTypeREV {
					out.Write(rpio.High)
				} else {
					out.Write(rpio.Low)
				}
			}
		}
	}
}

func clientSide(ctx context.Context, out rpio.Pin, ch chan rpio.State, appliance ApplianceData) {
	host := config.Host.Addr
	ipv4 := host
	iptable := make(map[string]string)
	for {
		select {
		case v := <-ch:
			fmt.Println(appliance.Name, v)
		GET_IP:
			ok := false
			if strings.HasSuffix(config.Host.Addr, ".local") {
				if ipv4, ok = iptable[host]; !ok && strings.HasSuffix(config.Host.Addr, ".local") {
					result := make(chan string)
					entriesCh := make(chan *mdns.ServiceEntry, 10)
					go func() {
						for e := range entriesCh {
							entry := e
							if (*entry).Host == host+"." {
								if entry.AddrV4 != nil {
									result <- entry.AddrV4.String()
								}
							}
						}
					}()
					mdns.Lookup("_http._tcp", entriesCh)
					select {
					case ipv4 = <-result:
					case <-time.After(time.Second):
						ipv4 = host
					}
					close(entriesCh)
					iptable[host] = ipv4
				}
			}
			switch appliance.Trigger {
			case TriggerTOGGLE:
				if v == rpio.Low {
					continue
				}
				status := getStatusFromHost(ipv4+":"+config.Host.Port, appliance.ID)
				var err error
				if status == "0" {
					appliance.sender.On(ctx)
					if err != nil {
						delete(iptable, host)
						goto GET_IP
					}
					if *appliance.StatusType == StatusTypeREV {
						out.Write(rpio.Low)
					} else {
						out.Write(rpio.High)
					}
				} else {
					appliance.sender.Off(ctx)
					if err != nil {
						delete(iptable, host)
						goto GET_IP
					}
					if *appliance.StatusType == StatusTypeREV {
						out.Write(rpio.High)
					} else {
						out.Write(rpio.Low)
					}
				}
			}
		}
	}
}
