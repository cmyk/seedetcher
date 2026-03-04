package printer

import (
	"math"
	"testing"

	"github.com/kortschak/qr"
)

func TestDescriptorSingleQRPlacement_Default(t *testing.T) {
	x, y, size := descriptorSingleQRPlacement(0)
	if size != descriptorSingleQRLayout.DefaultSize {
		t.Fatalf("size=%v want %v", size, descriptorSingleQRLayout.DefaultSize)
	}
	// Default anchor may clamp when default size would exceed plate bounds.
	expX, expY := descriptorSingleQRLayout.QRXMM, descriptorSingleQRLayout.QRYMM
	if expX+size > plateSizeMM {
		expX = plateSizeMM - size
	}
	if expY+size > plateSizeMM {
		expY = plateSizeMM - size
	}
	if x != expX || y != expY {
		t.Fatalf("xy=(%v,%v) want (%v,%v)", x, y, expX, expY)
	}
}

func TestDescriptorSingleQRPlacement_Clamped(t *testing.T) {
	x, y, size := descriptorSingleQRPlacement(200)
	if size != 200 {
		t.Fatalf("size=%v want 200", size)
	}
	if x+size != plateSizeMM || y+size != plateSizeMM {
		t.Fatalf("placement not clamped to plate bounds: x=%v y=%v size=%v", x, y, size)
	}
}

func TestDescriptorDualQRPlacement(t *testing.T) {
	l, err := qr.Encode("UR:CRYPTO-OUTPUT/1-6/AAAAAAAAAAAAAAAAAAAAAAAAAAAA", descriptorQRECC)
	if err != nil {
		t.Fatalf("encode left: %v", err)
	}
	r, err := qr.Encode("UR:CRYPTO-OUTPUT/2-6/BBBBBBBBBBBBBBBBBBBBBBBBBBBB", descriptorQRECC)
	if err != nil {
		t.Fatalf("encode right: %v", err)
	}
	size, leftX, rightX, y := descriptorDualQRPlacement(l, r)
	if size <= 0 {
		t.Fatalf("size=%v", size)
	}
	if leftX != 0 {
		t.Fatalf("leftX=%v want 0", leftX)
	}
	if rightX <= leftX {
		t.Fatalf("rightX=%v <= leftX=%v", rightX, leftX)
	}
	if rightX+size > plateSizeMM+1e-6 {
		t.Fatalf("right qr spills plate: rightX=%v size=%v", rightX, size)
	}
	if math.Abs((y+size)-plateSizeMM) > 1e-6 {
		t.Fatalf("bottom alignment broken: y=%v size=%v", y, size)
	}
	// Shared quiet-zone collapse: second QR starts before full-size offset.
	if !(rightX < size) {
		t.Fatalf("expected shared quiet-zone overlap, rightX=%v size=%v", rightX, size)
	}
}
