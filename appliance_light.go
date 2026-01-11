package controlremo

import (
	"context"

	"github.com/cormoran/natureremo"
)

type ApplianceLight struct {
	ApplianceData
	OnButton  *string `yaml:"OnButton"`
	OffButton *string `yaml:"OffButton"`
	Status    *bool   // true is power on
}

func (a ApplianceLight) On(ctx context.Context) (*natureremo.LightState, error) {
	if a.OnButton == nil {
		return a.Send(ctx, "on")
	} else {
		return a.Send(ctx, *a.OnButton)
	}
}

func (a ApplianceLight) Off(ctx context.Context) (*natureremo.LightState, error) {
	if a.OffButton == nil {
		return a.Send(ctx, "off")
	} else {
		return a.Send(ctx, *a.OffButton)
	}
}

func (a ApplianceLight) Send(ctx context.Context, button string) (*natureremo.LightState, error) {
	return remoClient.ApplianceService.SendLightSignal(ctx, &natureremo.Appliance{ID: a.ID}, button)
}

func (a ApplianceLight) Show() {
	// No GPIO operations - status is only maintained in memory
}

func (a *ApplianceLight) Set(value string) {
	r := value == "on"
	a.Status = &r
}

func (a *ApplianceLight) Get() (err error) {
	// Status is maintained in memory only, no external server requests
	return nil
}
