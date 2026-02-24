//go:build !linux || !arm

package main

import (
	"errors"
	"image"
	"image/draw"
	"io"
	"log"
	"time"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/bip39"
	"seedetcher.com/gui"
	"seedetcher.com/printer"
)

type Platform struct{}

func (p *Platform) Printer() io.Writer {
	log.Println("Printer is not implemented on this platform")
	return nil
}

func (p *Platform) PrinterStatus() (bool, string) {
	return false, ""
}

func Init() (*Platform, error) {
	log.Println("Running platform_dummy.go") // Add this to platform_dummy.go
	return new(Platform), nil
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

func (p *Platform) PrepareHBPForSDRemoval() error {
	return errors.New("Brother HBP runtime prep is not supported on this platform")
}

func (p *Platform) PrepareSDForRemoval() error {
	return nil
}

func (p *Platform) CreatePlates(ctx *gui.Context, mnemonic bip39.Mnemonic, desc *urtypes.OutputDescriptor, keyIdx int, paper printer.PaperSize, opts printer.RasterOptions) error {
	return errors.New("CreatePlates not implemented")
}
