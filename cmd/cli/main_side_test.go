package main

import (
	"testing"

	"seedetcher.com/printer"
)

func TestFilterScenesForSide(t *testing.T) {
	doc := &printer.PlateDocument{
		Version: "v1",
		Scenes: []printer.PlateScene{
			{Name: "seed_01"},
			{Name: "seed_02"},
			{Name: "desc_01"},
		},
	}

	seedDoc, err := filterScenesForSide(doc, "seed")
	if err != nil {
		t.Fatalf("seed filter: %v", err)
	}
	if got := len(seedDoc.Scenes); got != 2 {
		t.Fatalf("seed scenes=%d want 2", got)
	}

	descDoc, err := filterScenesForSide(doc, "desc")
	if err != nil {
		t.Fatalf("desc filter: %v", err)
	}
	if got := len(descDoc.Scenes); got != 1 {
		t.Fatalf("desc scenes=%d want 1", got)
	}

	bothDoc, err := filterScenesForSide(doc, "both")
	if err != nil {
		t.Fatalf("both filter: %v", err)
	}
	if got := len(bothDoc.Scenes); got != 3 {
		t.Fatalf("both scenes=%d want 3", got)
	}
}

func TestFilterScenesForSide_Invalid(t *testing.T) {
	doc := &printer.PlateDocument{Version: "v1", Scenes: []printer.PlateScene{{Name: "seed_01"}}}
	if _, err := filterScenesForSide(doc, "nope"); err == nil {
		t.Fatalf("expected invalid side error")
	}
}
