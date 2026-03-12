package testutils

import (
	"fmt"
	"strings"
)

func fixtureGroups() []struct {
	name     string
	fixtures []string
} {
	return []struct {
		name     string
		fixtures []string
	}{
		{
			name: "seed-only",
			fixtures: []string{
				"seed-12",
				"seed-15",
				"seed-18",
				"seed-21",
			},
		},
		{
			name: "singlesig",
			fixtures: []string{
				"singlesig",
				"singlesig-longwords",
				"singlesig-nested-p2sh-p2wpkh",
			},
		},
		{
			name: "multisig",
			fixtures: []string{
				"multisig",
				"multisig-mainnet-2of3",
				"multisig-nested-2of3",
				"multisig-2of2",
				"multisig-2of4",
				"multisig-3of4",
				"multisig-3of5",
				"multisig-4of7",
				"multisig-5of7",
				"multisig-7of10",
			},
		},
	}
}

func FixtureListText() string {
	var b strings.Builder
	b.WriteString("Available fixtures:\n")
	for _, g := range fixtureGroups() {
		b.WriteString(fmt.Sprintf("- %s:\n", g.name))
		for _, name := range g.fixtures {
			if _, ok := WalletConfigs[name]; !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
