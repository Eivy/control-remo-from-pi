package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/tenntenn/natureremo"
)

type ApplianceLight struct {
	ApplianceData
	OnButton  *string `yaml:"OnButton"`
	OffButton *string `yaml:"OffButton"`
	Status    *bool   // true is power on
}

func (a ApplianceLight) On(ctx context.Context) {
	if a.OnButton == nil {
		a.Send(ctx, "on")
	} else {
		a.Send(ctx, *a.OnButton)
	}
}

func (a ApplianceLight) Off(ctx context.Context) {
	if a.OffButton == nil {
		a.Send(ctx, "off")
	} else {
		a.Send(ctx, *a.OffButton)
	}
}

func (a ApplianceLight) Send(ctx context.Context, button string) {
	remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: a.ID}, button)
}

func (a ApplianceLight) Show() {
	// No GPIO operations - status is only maintained in memory
}

func (a *ApplianceLight) Set(value string) {
	r := value == "on"
	a.Status = &r
}

func (a *ApplianceLight) Get() (err error) {
	res, err := http.DefaultClient.Get(fmt.Sprintf("http://%s:%s/?id=%s", config.Host.Addr, config.Host.Port, a.ID))
	if err != nil {
		return
	}
	b, _ := io.ReadAll(res.Body)
	defer res.Body.Close()
	s := string(b) == "on"
	a.Status = &s
	return
}
