package gui

import (
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui/op"
)

// WalletConfirmScreen wraps descriptor confirmation.
type WalletConfirmScreen struct {
	Theme        *Colors
	Descriptor   urtypes.OutputDescriptor
	Mnemonic     bip39.Mnemonic
	OnDone       func(keyIdx int, ok bool) Screen
	AllowRestart func() Screen
}

func (s *WalletConfirmScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	ds := &DescriptorScreen{Descriptor: s.Descriptor, Mnemonic: s.Mnemonic}
	keyIdx, ok := ds.Confirm(ctx, ops, th)
	if !ok {
		if s.AllowRestart != nil {
			return s.AllowRestart()
		}
		return &MainMenuScreen{}
	}
	if s.OnDone != nil {
		return s.OnDone(keyIdx, true)
	}
	return &MainMenuScreen{}
}
