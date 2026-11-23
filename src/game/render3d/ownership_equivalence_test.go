package render3d

import (
	"bytes"
	"gunship2000-assets/internal/rasterlayout"
	"testing"
)

// V2.2 scalar reference. Keep the independent per-physical-pixel test.
func (o *spanOwnership) frozenSpan(dst []byte, y, l, r int, word uint16) {
	if o == nil || len(o.mask) != len(dst) {
		return
	}
	scale := 1
	if len(dst) == Width*Height*4 {
		scale = 2
	}
	door := sourceScreenDoorSpan(y, l, r, scale)
	layout := rasterlayout.FromPixels(len(dst))
	high := layout.Mapped()
	if high {
		door = sourceScreenDoorSpan(layout.LogicalY[y], layout.LogicalX[l], layout.LogicalX[r], 1)
	}
	base := y * layout.Width
	owner := byte(SurfaceOpaque)
	if o.material.role == EUROFieldTop || o.fallback {
		if o.material.owner != 0 {
			owner = o.material.owner
		}
	}
	if o.sparse && owner != byte(SurfaceBrown) && owner != byte(SurfaceFallback84) {
		owner = 0
	}
	// Ordinary opaque spans have one writer for their entire run. Resolve it
	// once, rather than re-testing material roles for every physical subpixel.
	if word != 0 && word != 0xffff && byte(word) != 0 {
		// The canonical footprint supplies the exact EURO6 pad correction.
		// Only its brown base needs a shade here; ordinary styles draw below.
		if owner == byte(SurfaceBrown) && o.feedback != nil {
			mappedFieldSpan(o.mask[base:], y, l, r, layout, brownSpan, o.feedback)
			return
		}
		s := o.mask[base+l : base+r+1]
		if owner == 0 {
			clear(s)
			return
		}
		for i := range s {
			s[i] = owner
		}
		return
	}
	for x := l; x <= r; x++ {
		at := base + x
		write := true
		if word == 0 {
			write = dst[at] >= 0x60
		} else if word == 0xffff {
			write = dst[at]&0xf0 == 0x60
		} else if byte(word) == 0 {
			sample := x
			if high {
				sample = layout.LogicalX[x]
			}
			write = door.writes(sample)
		}
		if o.material.role == GroundContactPad {
			if layer := SurfaceOwner(o.mask[at]).WithPad(write); layer != 0 {
				o.mask[at] = byte(layer)
				continue
			}
		}
		if !write {
			continue
		}
		o.mask[at] = owner
	}
}

func TestOwnershipSpanEquivalence(t *testing.T) {
	for _, res := range []string{"320x200", "640x400", "640x480", "800x600", "1024x768", "1280x1024"} {
		layout := rasterlayout.ByName(res)
		dst := make([]byte, layout.Width*layout.Height)
		want, got := make([]byte, len(dst)), make([]byte, len(dst))
		before, after := spanOwnership{mask: want}, spanOwnership{mask: got}
		for _, y := range []int{layout.Top, layout.Y[73] + 1} {
			off := layout.Width * y
			for x := 0; x < layout.Width; x++ {
				dst[off+x] = byte(7 * x)
				want[off+x] = byte(x)
				got[off+x] = byte(x)
			}
			for _, material := range []materialBinding{{role: GroundContactPad}, {}, {role: EUROFieldTop, owner: byte(SurfaceBrown)}, {role: GroundContactPad}} {
				before.material, after.material = material, material
				for _, word := range []uint16{0xec00, 0, 0xffff, 0x7f7f, 0x00ff, 0xec00} {
					for _, left := range []int{0, 1, 2, layout.X[131] - 1, layout.Width - 11} {
						for _, length := range []int{1, 2, 3, 8, 11} {
							right := left + length - 1
							before.frozenSpan(dst, y, left, right, word)
							after.span(dst, y, left, right, word)
							if !bytes.Equal(want[off:off+layout.Width], got[off:off+layout.Width]) {
								t.Fatalf("%s y=%d [%d,%d] word=%x role=%v", res, y, left, right, word, material.role)
							}
						}
					}
				}
			}
		}
	}
}
