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

// WritePSPlates composes seed/descriptor plates directly into a PostScript job
// without materializing all full-page bitmaps in memory at once.
// extraPages can be used to append already-rendered full pages (e.g. stats page).
func WritePSPlates(w io.Writer, seedPlates, descPlates []*image.Paletted, paper PaperSize, dpi float64, extraPages []*image.Paletted, progress ProgressFunc) error {
	plan, err := buildPlacementPlan(seedPlates, descPlates, paper, dpi, progress)
	if err != nil {
		return err
	}
	if len(plan.pages) == 0 && len(extraPages) == 0 {
		return fmt.Errorf("no pages to write")
	}
	pageWmm, pageHmm, ok := paperDimsMM(paper)
	if !ok {
		return fmt.Errorf("unsupported paper size: %v", paper)
	}
	totalBytes, err := estimatePSBytesForPlan(plan, pageWmm, pageHmm, extraPages)
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
	totalPages := len(plan.pages) + len(extraPages)
	if _, err := fmt.Fprintf(bw, "%%%%Pages: %d\n", totalPages); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(bw, "%%%%EndComments\n"); err != nil {
		return err
	}

	pageWpt := pageWmm * 72.0 / 25.4
	pageHpt := pageHmm * 72.0 / 25.4

	width := plan.pageWpx
	height := plan.pageHpx
	rowBytes := 0
	if width > 0 {
		rowBytes = (width + 7) / 8
	}
	rowPix := make([]uint8, width)
	rowPacked := make([]byte, rowBytes)
	hexRow := make([]byte, rowBytes*2+1)

	pageNum := 1
	for _, page := range plan.pages {
		if _, err := fmt.Fprintf(bw, "%%%%Page: %d %d\n", pageNum, pageNum); err != nil {
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

		for y := 0; y < height; y++ {
			for i := range rowPix {
				rowPix[i] = 0
			}
			for _, slot := range page.slots {
				if slot.plate == nil {
					continue
				}
				pb := slot.plate.Bounds()
				ly := y - slot.y
				if ly < 0 || ly >= pb.Dy() {
					continue
				}
				srcRow := slot.plate.Pix[(pb.Min.Y+ly)*slot.plate.Stride+pb.Min.X : (pb.Min.Y+ly)*slot.plate.Stride+pb.Min.X+pb.Dx()]
				copy(rowPix[slot.x:slot.x+pb.Dx()], srcRow)
			}
			packBits(rowPacked, rowPix)
			hex.Encode(hexRow[:rowBytes*2], rowPacked)
			hexRow[rowBytes*2] = '\n'
			if _, err := bw.Write(hexRow); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(bw, "grestore\nshowpage\n"); err != nil {
			return err
		}
		pageNum++
	}

	for _, page := range extraPages {
		if page == nil {
			return fmt.Errorf("extra page is nil")
		}
		b := page.Bounds()
		if b.Dx() != width || b.Dy() != height {
			return fmt.Errorf("extra page has unexpected dimensions: got %dx%d want %dx%d", b.Dx(), b.Dy(), width, height)
		}
		if _, err := fmt.Fprintf(bw, "%%%%Page: %d %d\n", pageNum, pageNum); err != nil {
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
		for y := 0; y < height; y++ {
			pix := page.Pix[(b.Min.Y+y)*page.Stride+b.Min.X : (b.Min.Y+y)*page.Stride+b.Min.X+width]
			packBits(rowPacked, pix)
			hex.Encode(hexRow[:rowBytes*2], rowPacked)
			hexRow[rowBytes*2] = '\n'
			if _, err := bw.Write(hexRow); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(bw, "grestore\nshowpage\n"); err != nil {
			return err
		}
		pageNum++
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

func estimatePSBytesForPlan(plan placementPlan, pageWmm, pageHmm float64, extraPages []*image.Paletted) (int64, error) {
	if plan.pageWpx <= 0 || plan.pageHpx <= 0 {
		return 0, fmt.Errorf("invalid page dimensions")
	}
	total := int64(0)
	pjlHeader := []byte("\x1b%-12345X@PJL JOB NAME=\"SE-PS\"\r\n@PJL SET PERSONALITY=POSTSCRIPT\r\n@PJL ENTER LANGUAGE=POSTSCRIPT\r\n")
	pjlFooter := []byte("\n\x1b%-12345X@PJL EOJ NAME=\"SE-PS\"\r\n\x1b%-12345X")
	total += int64(len(pjlHeader))
	total += int64(len("%!PS-Adobe-3.0\n"))
	totalPages := len(plan.pages) + len(extraPages)
	total += int64(len(fmt.Sprintf("%%%%Pages: %d\n", totalPages)))
	total += int64(len("%%EndComments\n"))
	pageWpt := pageWmm * 72.0 / 25.4
	pageHpt := pageHmm * 72.0 / 25.4

	width := plan.pageWpx
	height := plan.pageHpx
	rowBytes := (width + 7) / 8
	perPage := int64(0)
	perPage += int64(len(fmt.Sprintf("%%%%Page: %d %d\n", 1, 1)))
	perPage += int64(len(fmt.Sprintf("<< /PageSize [%.2f %.2f] >> setpagedevice\n", pageWpt, pageHpt)))
	perPage += int64(len("gsave\n"))
	perPage += int64(len("0 setgray\n"))
	perPage += int64(len(fmt.Sprintf("%.4f %.4f scale\n", pageWpt, pageHpt)))
	perPage += int64(len(fmt.Sprintf("/picstr %d string def\n", rowBytes)))
	perPage += int64(len(fmt.Sprintf("%d %d true\n", width, height)))
	perPage += int64(len(fmt.Sprintf("[%d 0 0 -%d 0 %d]\n", width, height, height)))
	perPage += int64(len("{ currentfile picstr readhexstring pop } imagemask\n"))
	perPage += int64(height * (rowBytes*2 + 1))
	perPage += int64(len("grestore\nshowpage\n"))
	total += int64(totalPages) * perPage
	total += int64(len("%%EOF\n"))
	total += int64(len(pjlFooter))
	return total, nil
}
