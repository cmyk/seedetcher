//go:build !linux || !arm

package main

import (
	"errors"
	"image"
	"image/draw"
	"log"
	"time"

	"seedetcher.com/backup"
	"seedetcher.com/engrave"
	"seedetcher.com/gui"
)

type Platform struct{}

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
