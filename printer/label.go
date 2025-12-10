package printer

import "sync"

var (
	labelOnce sync.Once
	labelVal  = "SEEDETCHER"
)

// walletLabel returns the footer label for plates.
func walletLabel() string {
	labelOnce.Do(func() {
		if labelVal == "" {
			labelVal = "SEEDETCHER"
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
