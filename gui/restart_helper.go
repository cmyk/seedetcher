package gui

import (
	"seedetcher.com/gui/assets"
	"seedetcher.com/gui/op"
)

// maybeRestart shows a restart/clear warning. Returns true if user confirms and calls reset().
// If the warning is declined, returns false and leaves state untouched.
func maybeRestart(ctx *Context, ops op.Ctx, th *Colors, reset func()) bool {
	confirm := &ConfirmWarningScreen{
		Title: "Restart Process?",
		Body:  "Do you want to restart and clear all scanned data?\n\nHold button to confirm.",
		Icon:  assets.IconDiscard,
	}
	if confirmWarning(ctx, ops, th, confirm) {
		reset()
		return true
	}
	return false
}
