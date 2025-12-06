package gui

import (
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/op"
	"seedetcher.com/printer"
)

// PrintFlowScreen wraps printing with retry/back handling.
type PrintFlowScreen struct {
	Theme      *Colors
	Mnemonic   bip39.Mnemonic
	Descriptor *urtypes.OutputDescriptor
	KeyIdx     int
	OnSuccess  func() Screen
	OnRetry    func() Screen
}

func (s *PrintFlowScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	printScreen := &PrintSeedScreen{}
	if printScreen.Print(ctx, ops, th, s.Mnemonic, s.Descriptor, s.KeyIdx, printer.PaperA4) {
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
