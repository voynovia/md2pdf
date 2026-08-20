package mdtopdf

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestInvertLightness(t *testing.T) {
	// Создаём тестовое изображение 100x100: левая половина белая, правая — красная.
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			if x < 50 {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255}) // белый
			} else {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255}) // красный
			}
		}
	}

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "test.png")
	dstPath := filepath.Join(tmpDir, "dark.png")

	f, err := os.Create(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	if err := invertLightness(srcPath, dstPath); err != nil {
		t.Fatalf("invertLightness: %v", err)
	}

	// Проверяем результат.
	df, err := os.Open(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()

	dst, _, err := image.Decode(df)
	if err != nil {
		t.Fatalf("decode dark image: %v", err)
	}

	// Белый → чёрный (L=1 → L=0).
	r, g, b, _ := dst.At(25, 50).RGBA()
	if r>>8 > 5 || g>>8 > 5 || b>>8 > 5 {
		t.Errorf("white should become black, got RGB(%d,%d,%d)", r>>8, g>>8, b>>8)
	}

	// Красный: H=0, S=1, L=0.5 → L'=0.5, цвет должен сохраниться.
	r, g, b, _ = dst.At(75, 50).RGBA()
	if r>>8 < 200 {
		t.Errorf("red hue should be preserved, R=%d", r>>8)
	}
	if g>>8 > 30 || b>>8 > 30 {
		t.Errorf("red should stay red, G=%d B=%d", g>>8, b>>8)
	}
}

func TestRGBToHSLRoundTrip(t *testing.T) {
	cases := []struct {
		r, g, b uint8
	}{
		{0, 0, 0},       // чёрный
		{255, 255, 255}, // белый
		{255, 0, 0},     // красный
		{0, 255, 0},     // зелёный
		{0, 0, 255},     // синий
		{128, 128, 128}, // серый
		{200, 100, 50},  // произвольный
	}

	for _, c := range cases {
		h, s, l := rgbToHSL(c.r, c.g, c.b)
		r2, g2, b2 := hslToRGB(h, s, l)

		dr := int(c.r) - int(r2)
		dg := int(c.g) - int(g2)
		db := int(c.b) - int(b2)

		if dr > 1 || dr < -1 || dg > 1 || dg < -1 || db > 1 || db < -1 {
			t.Errorf("roundtrip RGB(%d,%d,%d) → HSL(%.1f,%.3f,%.3f) → RGB(%d,%d,%d)",
				c.r, c.g, c.b, h, s, l, r2, g2, b2)
		}
	}
}
