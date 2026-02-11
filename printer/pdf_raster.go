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
