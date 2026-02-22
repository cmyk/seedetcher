package printer

import (
	"bufio"
	"encoding/hex"
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
	bw := bufio.NewWriterSize(pw, 128*1024)
	defer bw.Flush()

	pjlHeader := []byte("\x1b%-12345X@PJL JOB NAME=\"SE-PS\"\r\n@PJL SET PERSONALITY=POSTSCRIPT\r\n@PJL ENTER LANGUAGE=POSTSCRIPT\r\n")
	pjlFooter := []byte("\n\x1b%-12345X@PJL EOJ NAME=\"SE-PS\"\r\n\x1b%-12345X")
	if _, err := bw.Write(pjlHeader); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(bw, "%%!PS-Adobe-3.0\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(bw, "%%%%Pages: %d\n", len(pages)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(bw, "%%%%EndComments\n"); err != nil {
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
		if _, err := fmt.Fprintf(bw, "%%%%Page: %d %d\n", i+1, i+1); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "<< /PageSize [%.2f %.2f] >> setpagedevice\n", pageWpt, pageHpt); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "gsave\n"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "0 setgray\n"); err != nil {
			return err
		}
		// Render full-page image mask directly in page user space (points).
		if _, err := fmt.Fprintf(bw, "%.4f %.4f scale\n", pageWpt, pageHpt); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "/picstr %d string def\n", rowBytes); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "%d %d true\n", width, height); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "[%d 0 0 -%d 0 %d]\n", width, height, height); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(bw, "{ currentfile picstr readhexstring pop } imagemask\n"); err != nil {
			return err
		}
		hexRow := make([]byte, rowBytes*2+1)
		for y := 0; y < height; y++ {
			pix := page.Pix[y*page.Stride : y*page.Stride+width]
			packBits(buf, pix)
			hex.Encode(hexRow[:rowBytes*2], buf)
			hexRow[rowBytes*2] = '\n'
			if _, err := bw.Write(hexRow); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(bw, "grestore\nshowpage\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(bw, "%%%%EOF\n"); err != nil {
		return err
	}
	if _, err := bw.Write(pjlFooter); err != nil {
		return err
	}
	return nil
}

func estimatePSBytes(pages []*image.Paletted, pageWmm, pageHmm float64) (int64, error) {
	total := int64(0)
	pjlHeader := []byte("\x1b%-12345X@PJL JOB NAME=\"SE-PS\"\r\n@PJL SET PERSONALITY=POSTSCRIPT\r\n@PJL ENTER LANGUAGE=POSTSCRIPT\r\n")
	pjlFooter := []byte("\n\x1b%-12345X@PJL EOJ NAME=\"SE-PS\"\r\n\x1b%-12345X")
	total += int64(len(pjlHeader))
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
		total += int64(len(fmt.Sprintf("/picstr %d string def\n", rowBytes)))
		total += int64(len("{ currentfile picstr readhexstring pop } imagemask\n"))
		total += int64(height * (rowBytes*2 + 1)) // hex + newline
		total += int64(len("grestore\nshowpage\n"))
	}
	total += int64(len("%%EOF\n"))
	total += int64(len(pjlFooter))
	return total, nil
}
