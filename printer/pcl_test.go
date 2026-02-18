package printer

import (
	"bytes"
	"image"
	"testing"
)

func TestWritePCLPlatesMatchesComposedWritePCL(t *testing.T) {
	seedPlates := []*image.Paletted{
		testPlate(20, 20, 1),
		testPlate(20, 20, 2),
		testPlate(20, 20, 3),
	}
	descPlates := []*image.Paletted{
		testPlate(20, 20, 4),
		testPlate(20, 20, 5),
		testPlate(20, 20, 6),
	}
	const dpi = 10.0

	pages, err := ComposePages(seedPlates, descPlates, PaperLetter, dpi, nil)
	if err != nil {
		t.Fatalf("ComposePages: %v", err)
	}
	var composed bytes.Buffer
	if err := WritePCL(&composed, pages, dpi, PaperLetter, nil); err != nil {
		t.Fatalf("WritePCL: %v", err)
	}

	var direct bytes.Buffer
	if err := WritePCLPlates(&direct, seedPlates, descPlates, dpi, PaperLetter, nil); err != nil {
		t.Fatalf("WritePCLPlates: %v", err)
	}

	if !bytes.Equal(composed.Bytes(), direct.Bytes()) {
		t.Fatalf("PCL mismatch: composed=%d bytes direct=%d bytes", composed.Len(), direct.Len())
	}
}

func testPlate(w, h, pattern int) *image.Paletted {
	img := image.NewPaletted(image.Rect(0, 0, w, h), bwPalette)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if ((x+y)*pattern)%7 < 3 {
				img.Pix[y*img.Stride+x] = 1
			}
		}
	}
	return img
}
