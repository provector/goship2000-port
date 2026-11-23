package render3d

import (
	"image"
	"image/color"
	"testing"
)

func TestBrownFieldUsesExplicitPairPhase(t *testing.T) {
	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.RGBA{uint8(i), 0, 0, 255}
	}
	for _, scale := range []int{1, 2} {
		for _, field := range []struct {
			owner SurfaceOwner
			lo    byte
		}{{SurfaceBrown, 0xb7}, {SurfaceFallback7C, 0x7f}, {SurfaceFallback74, 0x77}} {
			w, h := Width*scale, Height*scale
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			ref := make([]byte, w*h)
			mask := make([]byte, len(ref))
			idx := make([]byte, len(ref))
			for y := 3; y < 11; y++ {
				fillSpan(ref, y, 3, 24, uint16(field.lo^7)<<8|uint16(field.lo), 1, nil)
				for x := 3; x <= 24; x++ {
					mask[y*w+x] = byte(field.owner)
					idx[y*w+x] = (field.lo & 0xfc) + byte(x%4)
				}
			}
			ResolveGroundPresentation(dst, idx, mask, pal, image.Rectangle{}, nil)
			for y := 3; y < 11; y++ {
				for x := 2; x <= 25; x++ {
					if got := dst.RGBAAt(x, y).R; got != ref[y*w+x] {
						t.Fatalf("scale%d (%d,%d) got%x want%x", scale, x, y, got, ref[y*w+x])
					}
				}
			}
		}
	}
}
