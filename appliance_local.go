package main

import (
	"context"

	"github.com/tenntenn/natureremo"
)

type ApplianceLocal struct {
	ApplianceData
	IP       string              `yaml:"IP"`
	OnLocal  natureremo.IRSignal `yaml:"OnLocal"`
	OffLocal natureremo.IRSignal `yaml:"OffLocal"`
}

func (a ApplianceLocal) On(ctx context.Context) {
	c := natureremo.NewLocalClient(a.IP)
	c.Emit(ctx, &a.OnLocal)
}

func (a ApplianceLocal) Off(ctx context.Context) {
	c := natureremo.NewLocalClient(a.IP)
	c.Emit(ctx, &a.OffLocal)
}

func (a ApplianceLocal) Send(ctx context.Context, button string) {
}
