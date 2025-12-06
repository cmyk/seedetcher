package gui

import "seedetcher.com/gui/op"

// Screen defines the minimal contract for a UI screen.
type Screen interface {
	// Update processes events and may return a new Screen (transition).
	Update(ctx *Context, ops op.Ctx) Screen
}
