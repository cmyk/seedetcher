//go:build !debug

package gui

func debugLoadTestWalletEnabled(ctx *Context) bool {
	_ = ctx
	return false
}

func newLoadTestWalletScreen(th *Colors) Screen {
	return &ActionChoiceScreen{Theme: th}
}
