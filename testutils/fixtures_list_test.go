package testutils

import (
	"strings"
	"testing"
)

func TestFixtureListTextContainsGroupsAndKeyFixtures(t *testing.T) {
	out := FixtureListText()
	required := []string{
		"Available fixtures:",
		"- seed-only:",
		"- singlesig:",
		"- multisig:",
		"seed-12",
		"singlesig-nested-p2sh-p2wpkh",
		"multisig-7of10",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("fixture list missing %q:\n%s", s, out)
		}
	}
}
