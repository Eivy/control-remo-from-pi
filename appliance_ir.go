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

func (a ApplianceIR) On(ctx context.Context) (*natureremo.LightState, error) {
	_, err := a.Send(ctx, a.OnSignal)
	return nil, err
}

func (a ApplianceIR) Off(ctx context.Context) (*natureremo.LightState, error) {
	_, err := a.Send(ctx, a.OffSignal)
	return nil, err
}

func (a ApplianceIR) Send(ctx context.Context, button string) (*natureremo.LightState, error) {
	err := remoClient.SignalService.Send(ctx, &natureremo.Signal{ID: button})
	return nil, err
}
