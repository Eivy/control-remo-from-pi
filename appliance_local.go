package controlremo

import (
	"context"

	"github.com/cormoran/natureremo"
)

type ApplianceLocal struct {
	ApplianceData
	IP       string              `yaml:"IP"`
	OnLocal  natureremo.IRSignal `yaml:"OnLocal"`
	OffLocal natureremo.IRSignal `yaml:"OffLocal"`
}

func (a ApplianceLocal) On(ctx context.Context) (*natureremo.LightState, error) {
	c := natureremo.NewLocalClient(a.IP)
	err := c.Emit(ctx, &a.OnLocal)
	return nil, err
}

func (a ApplianceLocal) Off(ctx context.Context) (*natureremo.LightState, error) {
	c := natureremo.NewLocalClient(a.IP)
	err := c.Emit(ctx, &a.OffLocal)
	return nil, err
}

func (a ApplianceLocal) Send(ctx context.Context, button string) (*natureremo.LightState, error) {
	return nil, nil
}
