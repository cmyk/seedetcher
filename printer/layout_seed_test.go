package printer

import "testing"

func TestDefaultSeedPlateLayout_Multisig(t *testing.T) {
	l := defaultSeedPlateLayout(3, false)
	if l.LeftColXMM != 10.0 || l.RightColXMM != 49.0 || l.QRLeftMM != 49.0 {
		t.Fatalf("unexpected multisig layout: %+v", l)
	}
}

func TestDefaultSeedPlateLayout_SinglesigVariant(t *testing.T) {
	l := defaultSeedPlateLayout(1, true)
	if l.LeftColXMM != 8.0 || l.RightColXMM != 47.0 || l.QRLeftMM != 47.0 {
		t.Fatalf("unexpected singlesig layout: %+v", l)
	}
}

func TestSeedQRYMM(t *testing.T) {
	y := seedQRYMM(seedQRSizeMM)
	want := plateSizeMM - seedQRYBottomMarginMM - seedQRSizeMM
	if y != want {
		t.Fatalf("seedQRYMM=%v want %v", y, want)
	}
}
