package main

import (
	"context"

	"github.com/cormoran/natureremo"
)

type ApplianceTV struct {
	ApplianceData
	OnButton  *string `yaml:"OnButton"`
	OffButton *string `yaml:"OffButton"`
}

func (a ApplianceTV) On(ctx context.Context) (*natureremo.LightState, error) {
	if a.OnButton == nil {
		return a.Send(ctx, "on")
	} else {
		return a.Send(ctx, *a.OnButton)
	}
}

func (a ApplianceTV) Off(ctx context.Context) (*natureremo.LightState, error) {
	if a.OffButton == nil {
		return a.Send(ctx, "off")
	} else {
		return a.Send(ctx, *a.OffButton)
	}
}

func (a ApplianceTV) Send(ctx context.Context, button string) (*natureremo.LightState, error) {
	_, err := remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: a.ID}, button)
	return nil, err
}
