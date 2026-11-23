package render3d

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// Low2 samples at logical addresses 0,2,4 are 1,0,3. Starting with
// DL/DH low bits 3/0 gives output pairs (2,3), (1,2), (0,1).
func TestFieldPatternHorizontalLogicalPairs(t *testing.T) {
	feedback := testTerrainFeedback(t)
	palette := make(color.Palette, 256)
	for i := range palette {
		palette[i] = color.RGBA{byte(i), 0, 0, 255}
	}
	for _, scale := range []int{1, 2} {
		for _, field := range []struct {
			owner SurfaceOwner
			base  byte
		}{{SurfaceBrown, 0xb4}, {SurfaceFallback74, 0x74}, {SurfaceFallback7C, 0x7c}} {
			width := Width * scale
			mask := make([]byte, width)
			indices := bytes.Repeat([]byte{field.base}, width)
			dst := image.NewRGBA(image.Rect(0, 0, width, 1))
			first, last := scale-1, 12*scale-1
			fieldFeedbackSpan(mask, 0, first, last, scale-1, fieldSpanToken(byte(field.owner)), feedback)
			ResolveGroundPresentation(dst, indices, mask, palette, image.Rectangle{}, feedback)
			want := []byte{2, 3, 1, 2, 0, 1}
			for x := 0; x < width; x++ {
				if x < first || x > last {
					if mask[x] != 0 {
						t.Fatal("span escaped clipping", x)
					}
				} else if got := dst.RGBAAt(x, 0).R; got != field.base+want[x/(2*scale)] {
					t.Fatalf("owner %x scale %d x %d got %x", field.owner, scale, x, got)
				}
			}
			// Moving only the right edge across a run changes span parity.
			// The odd leading seed is visible at the same screen coordinate.
			fieldFeedbackSpan(mask, 0, first, 10*scale-1, scale-1, fieldSpanToken(byte(field.owner)), feedback)
			if mask[first]&3 != 3 {
				t.Fatal("odd leading seed", mask[first])
			}
			for _, write := range []bool{false, true} {
				layer := SurfaceOwner(mask[first]).WithPad(write)
				if field.owner == SurfaceFallback74 {
					if layer != 0 {
						t.Fatal("unsupported underlay")
					}
					continue
				}
				base, written, ok := layer.Pad()
				if !ok || written != write || base != field.owner || layer&3 != 3 {
					t.Fatal("pad lost field shade or coverage", layer)
				}
			}
		}
	}
}
