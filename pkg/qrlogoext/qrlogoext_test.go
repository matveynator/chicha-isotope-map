//go:build !js

package qrlogoext

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodeTestPNG(t *testing.T, logoPNG []byte, opt Options) image.Image {
	t.Helper()

	var encoded bytes.Buffer
	if err := EncodePNG(
		&encoded,
		[]byte("https://example.test/radiation?lat=51.389&lon=30.099&zoom=13"),
		logoPNG,
		opt,
	); err != nil {
		t.Fatalf("EncodePNG() error = %v", err)
	}

	img, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode PNG: %v", err)
	}
	return img
}

func rgbaAt(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func solidPNG(t *testing.T, width, height int, col color.RGBA) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, col)
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode logo fixture: %v", err)
	}
	return encoded.Bytes()
}

func TestEncodePNGNoLogoKeepsQRMatrix(t *testing.T) {
	img := encodeTestPNG(t, nil, Options{
		TargetPx: 600,
		Fg:       color.RGBA{0, 0, 0, 255},
		Bg:       color.RGBA{255, 255, 255, 255},
		NoLogo:   true,
	})

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
			pixel := rgbaAt(img, x, y)
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

func TestEncodePNGDrawsVectorRadiationLogo(t *testing.T) {
	background := color.RGBA{255, 255, 255, 255}
	logoColor := color.RGBA{233, 192, 35, 255}
	img := encodeTestPNG(t, nil, Options{
		TargetPx:    600,
		Fg:          color.RGBA{0, 0, 0, 255},
		Bg:          background,
		Logo:        logoColor,
		LogoBoxFrac: 0.32,
	})

	cx := img.Bounds().Dx() / 2
	cy := img.Bounds().Dy() / 2
	if got := rgbaAt(img, cx, cy); got != logoColor {
		t.Fatalf("center pixel = %#v, want radiation logo color %#v", got, logoColor)
	}

	box := int(0.32 * float64(img.Bounds().Dx()))
	if box%2 == 1 {
		box--
	}
	x0 := cx - box/2
	y0 := cy - box/2
	if got := rgbaAt(img, x0+2, y0+2); got != background {
		t.Fatalf("logo box corner = %#v, want cleared background %#v", got, background)
	}
}

func TestEncodePNGDrawsEmbeddedLogoCenteredAndFitted(t *testing.T) {
	logoColor := color.RGBA{180, 20, 160, 255}
	background := color.RGBA{255, 255, 255, 255}
	logoPNG := solidPNG(t, 8, 4, logoColor)
	img := encodeTestPNG(t, logoPNG, Options{
		TargetPx:    600,
		Fg:          color.RGBA{0, 0, 0, 255},
		Bg:          background,
		Logo:        color.RGBA{1, 2, 3, 255},
		LogoBoxFrac: 0.32,
		LogoPadding: 24,
	})

	cx := img.Bounds().Dx() / 2
	cy := img.Bounds().Dy() / 2
	if got := rgbaAt(img, cx, cy); got != logoColor {
		t.Fatalf("embedded logo center = %#v, want %#v", got, logoColor)
	}

	// The 2:1 fixture must remain 2:1 after fitting into the square logo box.
	// With 192px box and 24px padding this yields 144x72, centered at the QR center.
	if got := rgbaAt(img, cx-71, cy-35); got != logoColor {
		t.Fatalf("scaled logo interior = %#v, want %#v", got, logoColor)
	}
	if got := rgbaAt(img, cx, cy-50); got != background {
		t.Fatalf("pixel outside fitted logo = %#v, want background %#v", got, background)
	}
}

func TestEncodePNGInvalidEmbeddedLogoFallsBackToVector(t *testing.T) {
	logoColor := color.RGBA{22, 133, 244, 255}
	img := encodeTestPNG(t, []byte("not a png"), Options{
		TargetPx:    500,
		Fg:          color.RGBA{0, 0, 0, 255},
		Bg:          color.RGBA{255, 255, 255, 255},
		Logo:        logoColor,
		LogoBoxFrac: 0.32,
	})

	cx := img.Bounds().Dx() / 2
	cy := img.Bounds().Dy() / 2
	if got := rgbaAt(img, cx, cy); got != logoColor {
		t.Fatalf("fallback logo center = %#v, want %#v", got, logoColor)
	}
}

func TestEncodePNGLogoPaddingCanSuppressEmbeddedImage(t *testing.T) {
	background := color.RGBA{255, 255, 255, 255}
	logoColor := color.RGBA{255, 0, 255, 255}
	img := encodeTestPNG(t, solidPNG(t, 3, 3, logoColor), Options{
		TargetPx:    300,
		Fg:          color.RGBA{0, 0, 0, 255},
		Bg:          background,
		LogoBoxFrac: 0.20,
		LogoPadding: 100,
	})

	cx := img.Bounds().Dx() / 2
	cy := img.Bounds().Dy() / 2
	if got := rgbaAt(img, cx, cy); got != background {
		t.Fatalf("suppressed embedded logo center = %#v, want background %#v", got, background)
	}
}

func TestEncodePNGDefaultsAndClampsOptions(t *testing.T) {
	img := encodeTestPNG(t, nil, Options{
		TargetPx:    240,
		LogoBoxFrac: 0.01,
		LogoPadding: -10,
	})
	if got := img.Bounds().Dx(); got != 240 {
		t.Fatalf("width = %d, want 240", got)
	}

	// Zero colors use defaults, and the center is the default black vector logo.
	cx := img.Bounds().Dx() / 2
	cy := img.Bounds().Dy() / 2
	if got := rgbaAt(img, cx, cy); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("default logo center = %#v, want black", got)
	}

	// Exercise the upper LogoBoxFrac clamp as well.
	img = encodeTestPNG(t, nil, Options{
		TargetPx:    240,
		LogoBoxFrac: 0.99,
	})
	if got := img.Bounds().Dx(); got != 240 {
		t.Fatalf("upper-clamp image width = %d, want 240", got)
	}
}

func TestFitRectPreservesAspectRatioAndHandlesZeroSource(t *testing.T) {
	if w, h := fitRect(8, 4, 144, 144); w != 144 || h != 72 {
		t.Fatalf("fitRect(8x4) = %dx%d, want 144x72", w, h)
	}
	if w, h := fitRect(0, 4, 30, 20); w != 30 || h != 20 {
		t.Fatalf("fitRect(zero width) = %dx%d, want max bounds", w, h)
	}
	if w, h := fitRect(1000, 1, 1, 1); w != 1 || h != 1 {
		t.Fatalf("fitRect tiny output = %dx%d, want 1x1", w, h)
	}
}
