package main

import (
	"context"

	"github.com/tenntenn/natureremo"
)

type ApplianceTV struct {
	ApplianceData
	OnButton  *string `yaml:"OnButton"`
	OffButton *string `yaml:"OffButton"`
}

func (a ApplianceTV) On(ctx context.Context) {
	if a.OnButton == nil {
		a.Send(ctx, "on")
	} else {
		a.Send(ctx, *a.OnButton)
	}
}

func (a ApplianceTV) Off(ctx context.Context) {
	if a.OffButton == nil {
		a.Send(ctx, "off")
	} else {
		a.Send(ctx, *a.OffButton)
	}
}

func (a ApplianceTV) Send(ctx context.Context, button string) {
	remoClient.ApplianceService.SendTVSignal(ctx, &natureremo.Appliance{ID: a.ID}, button)
}
