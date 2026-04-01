package mdtopdf

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	_ "image/gif"
	_ "image/jpeg"
)

// invertLightness создаёт копию изображения с инвертированной яркостью (HSL).
// Оттенок (H) и насыщенность (S) сохраняются, L' = 1 − L.
// Результат сохраняется в dstPath как PNG.
func invertLightness(srcPath, dstPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open %q: %w", srcPath, err)
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode %q: %w", srcPath, err)
	}

	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			// RGBA() возвращает pre-multiplied 16-bit значения, конвертируем в 8-bit.
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)
			a8 := uint8(a >> 8)

			h, s, l := rgbToHSL(r8, g8, b8)
			l = 1.0 - l
			nr, ng, nb := hslToRGB(h, s, l)

			dst.SetRGBA(x, y, color.RGBA{R: nr, G: ng, B: nb, A: a8})
		}
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", dstPath, err)
	}
	defer out.Close()

	if err := png.Encode(out, dst); err != nil {
		return fmt.Errorf("encode PNG %q: %w", dstPath, err)
	}

	return nil
}

// rgbToHSL конвертирует RGB [0,255] в HSL: H [0,360], S [0,1], L [0,1].
func rgbToHSL(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255.0
	gf := float64(g) / 255.0
	bf := float64(b) / 255.0

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	l = (maxC + minC) / 2.0

	if maxC == minC {
		return 0, 0, l // ахроматический
	}

	d := maxC - minC
	if l > 0.5 {
		s = d / (2.0 - maxC - minC)
	} else {
		s = d / (maxC + minC)
	}

	switch maxC {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6.0
		}
	case gf:
		h = (bf-rf)/d + 2.0
	case bf:
		h = (rf-gf)/d + 4.0
	}
	h *= 60.0

	return h, s, l
}

// hslToRGB конвертирует HSL в RGB [0,255].
func hslToRGB(h, s, l float64) (r, g, b uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255.0))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1.0 + s)
	} else {
		q = l + s - l*s
	}
	p := 2.0*l - q

	hNorm := h / 360.0

	r = uint8(math.Round(hueToRGB(p, q, hNorm+1.0/3.0) * 255.0))
	g = uint8(math.Round(hueToRGB(p, q, hNorm) * 255.0))
	b = uint8(math.Round(hueToRGB(p, q, hNorm-1.0/3.0) * 255.0))

	return r, g, b
}

// hueToRGB вспомогательная функция для конверсии HSL→RGB.
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1.0
	}
	if t > 1 {
		t -= 1.0
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6.0*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6.0
	default:
		return p
	}
}

// isBackgroundDark возвращает true если фон темы тёмный (яркость < 50%).
func (r *PdfRenderer) isBackgroundDark() bool {
	bg := r.BackgroundColor
	// Perceived brightness (ITU-R BT.601).
	lum := 0.299*float64(bg.Red) + 0.587*float64(bg.Green) + 0.114*float64(bg.Blue)
	return lum < 128
}
