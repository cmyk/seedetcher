package printer

import (
	"fmt"
	"image"
	"io"
)

// WritePS streams mono bitmaps as a PostScript job (one page per image).
// It prepends PJL language selection for printers that honor PJL over USB raw.
func WritePS(w io.Writer, pages []*image.Paletted, paper PaperSize, progress ProgressFunc) error {
	if len(pages) == 0 {
		return fmt.Errorf("no pages to write")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}
	totalBytes, err := estimatePSBytes(pages, pageWmm, pageHmm)
	if err != nil {
		return err
	}
	pw := newProgressWriter(StageSend, w, totalBytes, progress)
	if progress != nil && totalBytes > 0 {
		progress(StageSend, 0, totalBytes)
	}

	uel := []byte{0x1b, '%', '-', '1', '2', '3', '4', '5', 'X'}
	if _, err := pw.Write(uel); err != nil {
		return err
	}
	if _, err := pw.Write([]byte("@PJL ENTER LANGUAGE = POSTSCRIPT\r\n")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(pw, "%%!PS-Adobe-3.0\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(pw, "%%%%Pages: %d\n", len(pages)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(pw, "%%%%EndComments\n"); err != nil {
		return err
	}

	pageWpt := pageWmm * 72.0 / 25.4
	pageHpt := pageHmm * 72.0 / 25.4

	for i, page := range pages {
		if page == nil {
			return fmt.Errorf("page %d is nil", i)
		}
		b := page.Bounds()
		width, height := b.Dx(), b.Dy()
		if width <= 0 || height <= 0 {
			return fmt.Errorf("page %d has invalid dimensions", i)
		}
		rowBytes := (width + 7) / 8
		buf := make([]byte, rowBytes)
		if _, err := fmt.Fprintf(pw, "%%%%Page: %d %d\n", i+1, i+1); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(pw, "<< /PageSize [%.2f %.2f] >> setpagedevice\n", pageWpt, pageHpt); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(pw, "gsave\n"); err != nil {
			return err
		}
		// Render full-page image mask in pixel space.
		if _, err := fmt.Fprintf(pw, "%d %d scale\n", width, height); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(pw, "%d %d true\n", width, height); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(pw, "[%d 0 0 -%d 0 %d]\n", width, height, height); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(pw, "{ currentfile /ASCIIHexDecode filter } imagemask\n"); err != nil {
			return err
		}
		for y := 0; y < height; y++ {
			pix := page.Pix[y*page.Stride : y*page.Stride+width]
			packBits(buf, pix)
			for _, v := range buf {
				if _, err := fmt.Fprintf(pw, "%02X", v); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(pw, "\n"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(pw, ">\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(pw, "grestore\nshowpage\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(pw, "%%%%EOF\n"); err != nil {
		return err
	}
	if _, err := pw.Write(uel); err != nil {
		return err
	}
	return nil
}

func estimatePSBytes(pages []*image.Paletted, pageWmm, pageHmm float64) (int64, error) {
	total := int64(0)
	uel := []byte{0x1b, '%', '-', '1', '2', '3', '4', '5', 'X'}
	total += int64(len(uel))
	total += int64(len("@PJL ENTER LANGUAGE = POSTSCRIPT\r\n"))
	total += int64(len("%!PS-Adobe-3.0\n"))
	total += int64(len(fmt.Sprintf("%%%%Pages: %d\n", len(pages))))
	total += int64(len("%%EndComments\n"))
	pageWpt := pageWmm * 72.0 / 25.4
	pageHpt := pageHmm * 72.0 / 25.4

	for i, page := range pages {
		if page == nil {
			return 0, fmt.Errorf("page %d is nil", i)
		}
		b := page.Bounds()
		width, height := b.Dx(), b.Dy()
		if width <= 0 || height <= 0 {
			return 0, fmt.Errorf("page %d has invalid dimensions", i)
		}
		rowBytes := (width + 7) / 8
		total += int64(len(fmt.Sprintf("%%%%Page: %d %d\n", i+1, i+1)))
		total += int64(len(fmt.Sprintf("<< /PageSize [%.2f %.2f] >> setpagedevice\n", pageWpt, pageHpt)))
		total += int64(len("gsave\n"))
		total += int64(len(fmt.Sprintf("%d %d scale\n", width, height)))
		total += int64(len(fmt.Sprintf("%d %d true\n", width, height)))
		total += int64(len(fmt.Sprintf("[%d 0 0 -%d 0 %d]\n", width, height, height)))
		total += int64(len("{ currentfile /ASCIIHexDecode filter } imagemask\n"))
		total += int64(height * (rowBytes*2 + 1)) // hex + newline
		total += int64(len(">\n"))
		total += int64(len("grestore\nshowpage\n"))
	}
	total += int64(len("%%EOF\n"))
	total += int64(len(uel))
	return total, nil
}
