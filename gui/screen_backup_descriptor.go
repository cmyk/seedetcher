package gui

import (
	"fmt"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/gui/op"
	"seedetcher.com/logutil"
)

// DescriptorInputScreen wraps descriptor input as a Screen.
type DescriptorInputScreen struct {
	Theme  *Colors
	OnDone func(*urtypes.OutputDescriptor, bool) Screen
}

func (s *DescriptorInputScreen) Update(ctx *Context, ops op.Ctx) Screen {
	th := s.Theme
	if th == nil {
		th = &descriptorTheme
	}
	desc, ok := inputDescriptorFlow(ctx, ops, th)
	if s.OnDone != nil {
		return s.OnDone(desc, ok)
	}
	if !ok {
		return &MainMenuScreen{}
	}
	// Default: go back to menu.
	return &MainMenuScreen{}
}

func inputDescriptorFlow(ctx *Context, ops op.Ctx, th *Colors) (*urtypes.OutputDescriptor, bool) {
	originalDesc := ctx.LastDescriptor // Save original
	cs := &ChoiceScreen{
		Title:   "Descriptor",
		Lead:    "Choose input method",
		Choices: []string{"SCAN", "SKIP"},
	}
	if ctx.LastDescriptor != nil {
		cs.Choices = append(cs.Choices, "RE-USE")
	}
	for {
		choice, ok := cs.Choose(ctx, ops, th)
		if !ok {
			logutil.DebugLog("inputDescriptorFlow: Choose returned false")
			ctx.LastDescriptor = originalDesc // Restore on back
			return nil, false
		}
		switch choice {
		case 0: // Scan
			res, ok := (&ScanScreen{
				Title: "Scan",
				Lead:  "Descriptor",
			}).Scan(ctx, ops)
			if !ok {
				logutil.DebugLog("inputDescriptorFlow: Scan returned false")
				ctx.LastDescriptor = originalDesc // Restore on back
				return nil, false
			}
			desc, ok := res.(urtypes.OutputDescriptor)
			if !ok {
				logutil.DebugLog("inputDescriptorFlow: Scan result not descriptor")
				showError(ctx, ops, th, fmt.Errorf("Failed to parse descriptor"))
				continue
			}
			desc.Title = sanitizeTitle(desc.Title)
			logutil.DebugLog("inputDescriptorFlow: Returning desc with %d keys", len(desc.Keys))
			ctx.LastDescriptor = &desc
			return &desc, true
		case 1: // Skip
			logutil.DebugLog("inputDescriptorFlow: Skipping descriptor")
			ctx.LastDescriptor = originalDesc
			return nil, true
		case 2: // Re-use
			logutil.DebugLog("inputDescriptorFlow: Re-using last descriptor")
			return ctx.LastDescriptor, true
		}
	}
}
