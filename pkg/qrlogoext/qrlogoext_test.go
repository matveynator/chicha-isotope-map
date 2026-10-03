//go:build !js

package qrlogoext

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestEncodePNGNoLogoKeepsQRMatrix(t *testing.T) {
	var encoded bytes.Buffer
	err := EncodePNG(
		&encoded,
		[]byte("https://example.test/radiation?lat=51.389&lon=30.099&zoom=13"),
		nil,
		Options{
			TargetPx: 600,
			Fg:       color.RGBA{0, 0, 0, 255},
			Bg:       color.RGBA{255, 255, 255, 255},
			NoLogo:   true,
		},
	)
	if err != nil {
		t.Fatalf("EncodePNG() error = %v", err)
	}

	img, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	if got := img.Bounds().Dx(); got != 600 {
		t.Fatalf("width = %d, want 600", got)
	}
	if got := img.Bounds().Dy(); got != 600 {
		t.Fatalf("height = %d, want 600", got)
	}

	bounds := img.Bounds()
	margin := bounds.Dx() / 3
	black := color.RGBA{0, 0, 0, 255}
	white := color.RGBA{255, 255, 255, 255}
	blackCount := 0
	whiteCount := 0
	for y := bounds.Min.Y + margin; y < bounds.Max.Y-margin; y++ {
		for x := bounds.Min.X + margin; x < bounds.Max.X-margin; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			pixel := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
			switch pixel {
			case black:
				blackCount++
			case white:
				whiteCount++
			default:
				t.Fatalf("plain QR contains unexpected color %#v at %d,%d", pixel, x, y)
			}
		}
	}
	if blackCount == 0 || whiteCount == 0 {
		t.Fatalf("center QR matrix lost modules: black=%d white=%d", blackCount, whiteCount)
	}
}
