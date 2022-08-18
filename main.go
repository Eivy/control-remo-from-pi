package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
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
		go pinCheck(in, ch)
		if config.Host == nil {
			go serverSide(ctx, condition, out, ch, appliance)
		} else {
			go clientSide(ctx, condition, out, ch, appliance)
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

type getAppliancesParam struct {
	ctx        context.Context
	interval   time.Duration
	resultChan chan []*natureremo.Appliance
}

func getAppliances(param getAppliancesParam) {
	for range time.Tick(param.interval) {
		a, err := remoClient.ApplianceService.GetAll(param.ctx)
		if err != nil {
			continue
		}
		param.resultChan <- a
	}
}

type buttonParam struct {
	id     string
	button string
}

type gpioConfig struct {
	pinNumber int
	id        string
	button    [2]string
}

type checkInputGpioParam struct {
	targets []*gpioConfig
	changed chan *buttonParam
}

func checkInputGpio(param checkInputGpioParam) {
	for _, v := range param.targets {
		target := v
		pin := rpio.Pin(target.pinNumber)
		before := pin.Read()
		go func() {
			select {
			case <-time.Tick(time.Millisecond * 500):
				tmp := pin.Read()
				if before != tmp {
					param.changed <- &buttonParam{
						id:     target.id,
						button: target.button[tmp],
					}
					before = tmp
				}
			}
		}()
	}
}

func updateOutputGpio(appliance Appliance, newAppliance natureremo.Appliance) {
	statusFunc := func(status rpio.State) rpio.State {
		if appliance.StatusType == StatusTypeSTR {
			return status
		}
		return (status + 1) % 2
	}
	switch newAppliance.Type {
	case natureremo.ApplianceTypeAirCon:
		if newAppliance.AirConSettings.Button == "" {
			rpio.Pin(appliance.StatusPin).Write(statusFunc(1))
		} else {
			rpio.Pin(appliance.StatusPin).Write(statusFunc(0))
		}
	case natureremo.ApplianceTypeLight:
		if newAppliance.Light.State.Power == "on" {
			rpio.Pin(appliance.StatusPin).Write(statusFunc(1))
		} else {
			rpio.Pin(appliance.StatusPin).Write(statusFunc(0))
		}
	default:
		break
	}
}

func sendButton(ctx context.Context, appliance *Appliance, id, button string) {
	switch appliance.Type {
	case natureremo.ApplianceTypeLight:
		light := natureremo.Appliance{
			ID: appliance.ID,
		}
		result, err := remoClient.ApplianceService.SendLightSignal(ctx, &light, button)
		if err != nil {
			log.Println(err)
		}
		newAppliance := natureremo.Appliance{
			ID: appliance.ID,
			Light: &natureremo.Light{
				State: result,
			},
		}
		updateOutputGpio(*appliance, newAppliance)
	case natureremo.ApplianceTypeTV:
		break
	case natureremo.ApplianceTypeIR:
		break
	}
}

func getStatusFromHost(dist, id string) string {
	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s/?id=%s", dist, id))
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer res.Body.Close()
	b, _ := ioutil.ReadAll(res.Body)
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
		case natureremo.ApplianceTypeLight:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
			if button == "on" {
				if a.StatusType == StatusTypeSTR {
					rpio.Pin(a.StatusPin).Write(rpio.High)
				} else {
					rpio.Pin(a.StatusPin).Write(rpio.Low)
				}
			} else {
				if a.StatusType == StatusTypeSTR {
					rpio.Pin(a.StatusPin).Write(rpio.Low)
				} else {
					rpio.Pin(a.StatusPin).Write(rpio.High)
				}
			}
		case natureremo.ApplianceTypeTV:
			remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: id}, button)
		case natureremo.ApplianceTypeIR:
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
					err := remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OnSignal})
					if err != nil {
						return
					}
					rpio.Pin(a.StatusPin).Write(rpio.High)
				} else {
					remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: a.OffSignal})
					rpio.Pin(a.StatusPin).Write(rpio.Low)
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
	} else {
		host := config.Host.Addr
	LOOP:
		if strings.HasSuffix(config.Host.Addr, ".local") {
			entriesCh := make(chan *mdns.ServiceEntry, 4)
			result := make(chan string)
			go func() {
				for entry := range entriesCh {
					fmt.Println(entry)
					if (*entry).Host == config.Host.Addr+"." {
						if entry.AddrV4 == nil {
							continue
						}
						result <- fmt.Sprintf("%s:%d", (*entry).AddrV4.String(), (*entry).Port)
					}
				}
			}()
			mdns.Lookup("_http._tcp", entriesCh)
			timeout := time.After(time.Second)
			select {
			case host = <-result:
			case <-timeout:
				goto LOOP
			}
			close(entriesCh)
		}
		for {
			interval := time.Tick(time.Second * 5)
			select {
			case <-interval:
				for _, a := range config.Appliances {
					if a.Type == natureremo.ApplianceTypeIR {
						continue
					}
					res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s:%s/?id=%s", host, config.Host.Port, a.ID))
					if err != nil {
						goto LOOP
					}
					b, _ := ioutil.ReadAll(res.Body)
					res.Body.Close()
					s := string(b)
					fmt.Print(s)
					if s == "0" {
						if a.StatusType == StatusTypeSTR {
							rpio.Pin(a.StatusPin).Write(rpio.Low)
						} else {
							rpio.Pin(a.StatusPin).Write(rpio.High)
						}
					} else {
						if a.StatusType == StatusTypeSTR {
							rpio.Pin(a.StatusPin).Write(rpio.High)
						} else {
							rpio.Pin(a.StatusPin).Write(rpio.Low)
						}
					}
				}
			}
		}
	}
}

func pinCheck(in rpio.Pin, ch chan rpio.State) {
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
}

func serverSide(ctx context.Context, condition rpio.Pin, out rpio.Pin, ch chan rpio.State, appliance Appliance) {
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
				button := "on"
				if appliance.OnButton != "" {
					button = appliance.OnButton
				}
				switch appliance.Type {
				case natureremo.ApplianceTypeLight:
					remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, button)
				case natureremo.ApplianceTypeTV:
					remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, button)
				case natureremo.ApplianceTypeIR:
					remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: appliance.OnSignal})
				}
				if appliance.StatusType == StatusTypeREV {
					out.Write(rpio.Low)
				} else {
					out.Write(rpio.High)
				}
			} else {
				button := "off"
				if appliance.OffButton != "" {
					button = appliance.OffButton
				}
				switch appliance.Type {
				case natureremo.ApplianceTypeLight:
					remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, button)
				case natureremo.ApplianceTypeTV:
					remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: appliance.ID}, button)
				case natureremo.ApplianceTypeIR:
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
}

func clientSide(ctx context.Context, condition rpio.Pin, out rpio.Pin, ch chan rpio.State, appliance Appliance) {
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
					if appliance.OnButton != "" {
						err = sendButtonToHost(ipv4+":"+config.Host.Port, appliance.ID, appliance.OnButton)
					} else {
						err = sendButtonToHost(ipv4+":"+config.Host.Port, appliance.ID, "on")
					}
					if err != nil {
						delete(iptable, host)
						goto GET_IP
					}
					if appliance.StatusType == StatusTypeREV {
						out.Write(rpio.Low)
					} else {
						out.Write(rpio.High)
					}
				} else {
					if appliance.OffButton != "" {
						err = sendButtonToHost(ipv4+":"+config.Host.Port, appliance.ID, appliance.OffButton)
					} else {
						err = sendButtonToHost(ipv4+":"+config.Host.Port, appliance.ID, "off")
					}
					if err != nil {
						delete(iptable, host)
						goto GET_IP
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
}
