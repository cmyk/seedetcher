package printer

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"

	"github.com/jung-kurt/gofpdf/v2"
)

// WritePDFRaster writes precomposed raster pages into a PDF.
// Pages must already be laid out for the selected paper size.
func WritePDFRaster(w io.Writer, pages []*image.Paletted, paper PaperSize) error {
	if len(pages) == 0 {
		return fmt.Errorf("no pages to write")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: pageWmm, Ht: pageHmm},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)

	imgOpts := gofpdf.ImageOptions{
		ImageType: "PNG",
		ReadDpi:   false,
	}

	for i, page := range pages {
		if page == nil {
			return fmt.Errorf("page %d is nil", i)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, page); err != nil {
			return fmt.Errorf("encode page %d: %w", i, err)
		}
		name := fmt.Sprintf("page-%d", i)
		pdf.RegisterImageOptionsReader(name, imgOpts, bytes.NewReader(buf.Bytes()))
		pdf.AddPage()
		pdf.ImageOptions(name, 0, 0, pageWmm, pageHmm, false, imgOpts, 0, "")
	}

	if err := pdf.Output(w); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}

// WritePDFPlates writes seed/descriptor plate bitmaps into a PDF without
// materializing full-page raster images first.
func WritePDFPlates(w io.Writer, seedPlates, descPlates []*image.Paletted, paper PaperSize, dpi float64) error {
	plan, err := buildPlacementPlan(seedPlates, descPlates, paper, dpi, nil)
	if err != nil {
		return err
	}
	if len(plan.pages) == 0 {
		return fmt.Errorf("no pages to write")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: pageWmm, Ht: pageHmm},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)

	imgOpts := gofpdf.ImageOptions{
		ImageType: "PNG",
		ReadDpi:   false,
	}
	imgIdx := 0
	pxToMM := func(px int) float64 {
		return float64(px) * 25.4 / dpi
	}

	for _, page := range plan.pages {
		pdf.AddPage()
		for _, slot := range page.slots {
			if slot.plate == nil {
				continue
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, slot.plate); err != nil {
				return fmt.Errorf("encode plate image: %w", err)
			}
			name := fmt.Sprintf("plate-%d", imgIdx)
			imgIdx++
			pdf.RegisterImageOptionsReader(name, imgOpts, bytes.NewReader(buf.Bytes()))
			b := slot.plate.Bounds()
			pdf.ImageOptions(
				name,
				pxToMM(slot.x),
				pxToMM(slot.y),
				pxToMM(b.Dx()),
				pxToMM(b.Dy()),
				false,
				imgOpts,
				0,
				"",
			)
		}
	}

	if err := pdf.Output(w); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}
