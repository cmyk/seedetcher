package gui

import (
	"seedetcher.com/gui/op"
	"seedetcher.com/printer"
)

// PrintFlowScreen wraps printing with retry/back handling.
type PrintFlowScreen struct {
	Theme     *Colors
	Job       PrintJob
	OnSuccess func() Screen
	OnRetry   func() Screen
}

func (s *PrintFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	printScreen := &PrintSeedScreen{}
	if printScreen.Print(ctx, ops, th, s.Job.Mnemonic, s.Job.Descriptor, s.Job.KeyIdx, printer.PaperA4, s.Job.Label) {
		if s.OnSuccess != nil {
			return s.OnSuccess()
		}
		return &MainMenuScreen{}
	}
	if s.OnRetry != nil {
		return s.OnRetry()
	}
	return &MainMenuScreen{}
}
