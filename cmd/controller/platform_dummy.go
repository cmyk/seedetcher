//go:build !linux || !arm

package main

import (
	"errors"
	"image"
	"image/draw"
	"io"
	"log"
	"time"

	"seedetcher.com/backup"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/engrave"
	"seedetcher.com/gui"
	"seedetcher.com/printer"
)

type Platform struct{}

func (p *Platform) PrintPDF(mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paperFormat printer.PaperSize) error {
	log.Println("PrintPDF is not implemented on this platform")
	return errors.New("PrintPDF not supported in platform_dummy")
}

func (p *Platform) Printer() io.Writer {
	log.Println("Printer is not implemented on this platform")
	return nil
}

func Init() (*Platform, error) {
	log.Println("Running platform_dummy.go") // Add this to platform_dummy.go
	return new(Platform), nil
}

func (p *Platform) PlateSizes() []backup.PlateSize {
	return nil
}

func (p *Platform) EngraverParams() engrave.Params {
	return engrave.Params{}
}

func (p *Platform) Engraver() (gui.Engraver, error) {
	return nil, errors.New("Engraver not implemented")
}

func (p *Platform) DisplaySize() image.Point {
	return image.Pt(1, 1)
}

func (p *Platform) Dirty(r image.Rectangle) error {
	return nil
}

func (p *Platform) NextChunk() (draw.RGBA64Image, bool) {
	return nil, false
}

func (p *Platform) Wakeup() {
}

func (p *Platform) AppendEvents(deadline time.Time, evts []gui.Event) []gui.Event {
	return evts
}

func (p *Platform) CameraFrame(dims image.Point) {
}

func (p *Platform) ScanQR(img *image.Gray) ([][]byte, error) {
	return nil, errors.New("ScanQR not implemented")
}
