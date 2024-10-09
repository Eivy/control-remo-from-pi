package main

import (
	"context"

	"github.com/tenntenn/natureremo"
)

type ApplianceIR struct {
	ApplianceData
	OnSignal  string `yaml:"OnSignal"`
	OffSignal string `yaml:"OffSignal"`
}

func (a ApplianceIR) On(ctx context.Context) {
	a.Send(ctx, a.OnSignal)
}

func (a ApplianceIR) Off(ctx context.Context) {
	a.Send(ctx, a.OffSignal)
}

func (a ApplianceIR) Send(ctx context.Context, button string) {
	remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: button})
}
