package integration

import (
	"bytes"
	"image/png"
	"testing"
)

// The sampler wraps over 511 pixels: the supplied 512th edge duplicates the
// first. Validate the actual embedded art, including alpha and seamless edges.
func TestSurfaceTextureAssets125(t *testing.T) {
	for _, name := range []string{"grass", "dry", "sand", "rock", "snow", "volcanic"} {
		t.Run(name, func(t *testing.T) {
			data, err := testAssets.ReadFile("web/surface-ground-" + name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			im, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if im.Bounds().Dx() != 512 || im.Bounds().Dy() != 512 {
				t.Fatal("invalid material size")
			}
			same := func(x, y, xx, yy int) bool {
				r, g, b, a := im.At(x, y).RGBA()
				rr, gg, bb, aa := im.At(xx, yy).RGBA()
				return r == rr && g == gg && b == bb && a == aa
			}
			for i := 0; i < 512; i++ {
				if !same(0, i, 511, i) || !same(i, 0, i, 511) {
					t.Fatal("material edges do not wrap seamlessly", i)
				}
				for j := 0; j < 512; j++ {
					_, _, _, a := im.At(i, j).RGBA()
					if a != 65535 {
						t.Fatal("unexpected transparent ground")
					}
				}
			}
		})
	}
}
