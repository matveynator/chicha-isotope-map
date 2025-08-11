//go:build !js

package qrlogoext

// QR with big center logo using github.com/skip2/go-qrcode (ECC=H).
// - Yellow background (incl. quiet zone), black modules.
// - Large central box; logo drawn inside (PNG input) or fallback radiation mark.
// - No extra deps beyond go-qrcode; scaling is our own nearest-neighbor.
// - Concurrency is not needed: go-qrcode is fast; all drawing is in-memory.

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"

	qrcode "github.com/skip2/go-qrcode"
)

type Options struct {
	// Output size (px)
	TargetPx int

	// Colors
	Fg   color.RGBA // цвет модулей QR
	Bg   color.RGBA // фон (включая quiet-zone)
	Logo color.RGBA // цвет знака радиации (для векторного варианта)

	// Центральный квадрат под логотип (доля от края изображения 0.20..0.40)
	LogoBoxFrac float64

	// Отступ логотипа от краёв квадрата (px), если вставляем PNG
	LogoPadding int
}

func EncodePNG(w io.Writer, data []byte, logoPNG []byte, opt Options) error {
	// ---- defaults
	if opt.TargetPx <= 0 {
		opt.TargetPx = 1400
	}
	if opt.LogoPadding < 0 {
		opt.LogoPadding = 0
	}
	if opt.LogoBoxFrac <= 0 {
		opt.LogoBoxFrac = 0.32
	}
	if opt.LogoBoxFrac < 0.20 {
		opt.LogoBoxFrac = 0.20
	}
	if opt.LogoBoxFrac > 0.40 {
		opt.LogoBoxFrac = 0.40
	}
	if (opt.Fg == color.RGBA{}) {
		opt.Fg = color.RGBA{0, 0, 0, 255}
	}
	if (opt.Bg == color.RGBA{}) {
		opt.Bg = color.RGBA{0xE6, 0xC1, 0x37, 0xFF}
	} // можно переопределить в хендлере
	if (opt.Logo == color.RGBA{}) {
		opt.Logo = color.RGBA{0, 0, 0, 255}
	}

	// ---- build QR with ECC=H
	qr, err := qrcode.New(string(data), qrcode.Highest)
	if err != nil {
		return err
	}
	qr.ForegroundColor = opt.Fg
	qr.BackgroundColor = opt.Bg
	qr.DisableBorder = false

	src := qr.Image(opt.TargetPx)
	b := src.Bounds()
	W, H := b.Dx(), b.Dy()

	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{opt.Bg}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Over)

	// ---- central box
	box := int(opt.LogoBoxFrac * float64(min(W, H)))
	if box%2 == 1 {
		box--
	}
	if box < W/6 {
		box = W / 6
	}
	cx, cy := W/2, H/2
	x0 := cx - box/2
	y0 := cy - box/2
	fillRect(dst, x0, y0, box, box, opt.Bg)

	// ---- logo: PNG or vector fallback (now colored from opt.Logo)
	if len(logoPNG) > 0 {
		img, err := png.Decode(bytes.NewReader(logoPNG))
		if err == nil {
			pad := opt.LogoPadding
			maxW := box - 2*pad
			maxH := box - 2*pad
			if maxW > 0 && maxH > 0 {
				wr, hr := img.Bounds().Dx(), img.Bounds().Dy()
				sw, sh := fitRect(wr, hr, maxW, maxH)
				scaled := scaleNearest(img, sw, sh)
				ox := cx - sw/2
				oy := cy - sh/2
				draw.Draw(dst, image.Rect(ox, oy, ox+sw, oy+sh), scaled, image.Point{}, draw.Over)
			}
		} else {
			drawRadiation(dst, cx, cy, box, opt.Logo)
		}
	} else {
		drawRadiation(dst, cx, cy, box, opt.Logo)
	}

	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	return enc.Encode(w, dst)
}

// drawRadiation draws a colored radiation sign with longer blades and a ring.
func drawRadiation(dst *image.RGBA, cx, cy, box int, col color.RGBA) {
	half := box / 2

	// Longer blades: extend nearly to the box (keep tiny gap).
	rOuter := int(0.96 * float64(half)) // раньше было ~0.86
	rInner := int(0.35 * float64(rOuter))
	rCenter := int(0.20 * float64(rOuter))

	// Blades (three 60° wedges)
	drawWedge(dst, cx, cy, rInner, rOuter, deg(90-30), deg(90+30), col)
	drawWedge(dst, cx, cy, rInner, rOuter, deg(210-30), deg(210+30), col)
	drawWedge(dst, cx, cy, rInner, rOuter, deg(330-30), deg(330+30), col)

	// Center dot
	fillCircle(dst, cx, cy, rCenter, col)

}

// ---------- tiny raster helpers (nearest-neighbor scale, rect fill, primitives) ----------

func fitRect(w, h, maxW, maxH int) (int, int) {
	if w == 0 || h == 0 {
		return maxW, maxH
	}
	rx := float64(maxW) / float64(w)
	ry := float64(maxH) / float64(h)
	s := rx
	if ry < rx {
		s = ry
	}
	sw := int(math.Floor(float64(w) * s))
	sh := int(math.Floor(float64(h) * s))
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	return sw, sh
}

func scaleNearest(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw := sb.Dx()
	sh := sb.Dy()
	for y := 0; y < h; y++ {
		sy := sb.Min.Y + int(float64(y)*float64(sh)/float64(h))
		for x := 0; x < w; x++ {
			sx := sb.Min.X + int(float64(x)*float64(sw)/float64(w))
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func fillRect(img *image.RGBA, x, y, w, h int, col color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			img.Set(xx, yy, col)
		}
	}
}

func deg(d float64) float64 { return d * (math.Pi / 180) }

func fillCircle(img *image.RGBA, cx, cy, r int, col color.RGBA) {
	if r <= 0 {
		return
	}
	r2 := r * r
	b := img.Bounds()
	minY := max(cy-r, b.Min.Y)
	maxY := min(cy+r, b.Max.Y-1)
	for y := minY; y <= maxY; y++ {
		dy := y - cy
		xx := int(math.Sqrt(float64(r2 - dy*dy)))
		x1 := max(cx-xx, b.Min.X)
		x2 := min(cx+xx, b.Max.X-1)
		for x := x1; x <= x2; x++ {
			img.Set(x, y, col)
		}
	}
}

func drawWedge(img *image.RGBA, cx, cy, ri, ro int, a0, a1 float64, col color.RGBA) {
	if ro <= ri || ro <= 0 {
		return
	}
	for a0 < 0 {
		a0 += 2 * math.Pi
	}
	for a1 < 0 {
		a1 += 2 * math.Pi
	}
	for a0 >= 2*math.Pi {
		a0 -= 2 * math.Pi
	}
	for a1 >= 2*math.Pi {
		a1 -= 2 * math.Pi
	}
	in := func(a float64) bool {
		if a0 <= a1 {
			return a >= a0 && a <= a1
		}
		return a >= a0 || a <= a1
	}
	b := img.Bounds()
	minX := max(cx-ro, b.Min.X)
	maxX := min(cx+ro, b.Max.X-1)
	minY := max(cy-ro, b.Min.Y)
	maxY := min(cy+ro, b.Max.Y-1)
	ro2 := ro * ro
	ri2 := ri * ri
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			dx := x - cx
			dy := y - cy
			r2 := dx*dx + dy*dy
			if r2 < ri2 || r2 > ro2 {
				continue
			}
			a := math.Atan2(float64(dy), float64(dx))
			if a < 0 {
				a += 2 * math.Pi
			}
			if in(a) {
				img.Set(x, y, col)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
