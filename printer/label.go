package printer

import "sync"

const DefaultWalletLabel = "SEEDETCHER"

var (
	labelOnce sync.Once
	labelVal  = DefaultWalletLabel
)

// walletLabel returns the footer label for plates.
func walletLabel() string {
	labelOnce.Do(func() {
		if labelVal == "" {
			labelVal = DefaultWalletLabel
		}
	})
	return labelVal
}

// SetWalletLabel overrides the default label shown on plates.
func SetWalletLabel(s string) {
	if s == "" {
		return
	}
	labelVal = s
}
