package gui

import "seedetcher.com/gui/op"

// FlowScreen wraps the legacy mainFlow so we can gradually migrate to Screen-based navigation.
type FlowScreen struct{}

func (FlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	mainFlow(ctx, ops)
	return FlowScreen{}
}
