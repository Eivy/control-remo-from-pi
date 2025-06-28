package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/eivy/control-remo-from-pi/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tenntenn/natureremo"
)

var config Config
var timer = make(map[string]*time.Timer)
var metricsCollector *metrics.Collector

func main() {
	var err error
	config, err = ReadConfig()
	if err != nil {
		log.Fatal(err)
	}
	
	// Get Remo secret from environment variable
	remoSecret := os.Getenv("REMO_SECRET")
	if remoSecret == "" {
		log.Fatal("REMO_SECRET environment variable is required")
	}
	remoClient = natureremo.NewClient(remoSecret)
	
	// Initialize metrics collector
	metricsCollector = metrics.NewCollector(remoClient, 60*time.Second)
	prometheus.MustRegister(metricsCollector)
	
	ctx := context.Background()
	t := config.CheckInterval
	if t == 0 {
		t = 20
	}
	fmt.Printf("%#v\n", config.Server)
	if config.Server != nil {
		go statusCheck(&ctx, t)
		http.HandleFunc("/", remoControl)
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", config.Server.Port), nil)
	} else {
		statusCheck(&ctx, t)
	}
}



func remoControl(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	
	v := r.URL.Query()
	id := v.Get("id")
	button := v.Get("button")
	fmt.Printf("web requested: %s, %s\n", id, button)
	
	// Default to success, will be updated if error occurs
	statusCode := 200
	defer func() {
		if metricsCollector != nil {
			duration := time.Since(start).Seconds()
			metricsCollector.UpdateAPIMetrics("remoControl", statusCode, duration, nil)
		}
	}()
	if button != "" {
		ctx := context.Background()
		a, ok := config.Appliances[id]
		if !ok {
			fmt.Println("missing")
			statusCode = 404
			w.WriteHeader(404)
			return
		}
		a.sender.Send(ctx, button)
		t := a.Type
		switch t {
		case ApplianceTypeLight:
			a.sender.Send(ctx, button)
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
				} else {
					a.sender.Off(ctx)
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
		// Status check removed as GPIO functionality was removed
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func statusCheck(ctx *context.Context, intervalSec time.Duration) {
	interval := time.Tick(time.Second * intervalSec)
	for {
		select {
		case <-interval:
			start := time.Now()
			as, err := remoClient.ApplianceService.GetAll(*ctx)
			duration := time.Since(start).Seconds()
			
			if err != nil {
				log.Println(err)
				// Record API error metrics
				if metricsCollector != nil {
					metricsCollector.UpdateAPIMetrics("GetAll", 500, duration, nil)
				}
			} else {
				// Record successful API call metrics
				if metricsCollector != nil {
					metricsCollector.UpdateAPIMetrics("GetAll", 200, duration, nil)
				}
			}
			
			for _, a := range as {
				switch a.Type {
				case natureremo.ApplianceTypeLight:
					ap := config.Appliances[a.ID]
					ap.display.Set(a.Light.State.Power)
					ap.display.Show()
					
					// Update appliance metrics
					if metricsCollector != nil {
						powerState := a.Light.State.Power == "on"
						metricsCollector.UpdateApplianceState(a.ID, a.Nickname, "light", powerState)
					}
				}
			}
		}
	}
}



