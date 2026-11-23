package render3d

import (
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
)

// SurfaceOwner is the compact presentation discriminant carried beside indexed
// pixels. Pad values retain both the approved base and whether the source wrote
// this sample or left a transparent hole. Sky ownership keeps its separate A0s.
type SurfaceOwner byte

const (
	SurfaceNone       SurfaceOwner = 0
	SurfaceOpaque     SurfaceOwner = 1
	SurfaceBrown      SurfaceOwner = 0xb4
	SurfaceFieldSide  SurfaceOwner = 0x49
	SurfaceFieldTop   SurfaceOwner = 0x58
	SurfaceFallback74 SurfaceOwner = 0x5d
	SurfaceFallback7C SurfaceOwner = 0x5f
	SurfaceFallback84 SurfaceOwner = 0x61
)

var padLayers = [...]struct{ base, write, hole SurfaceOwner }{
	{SurfaceFallback7C, 0x5b, 0x5a}, {SurfaceBrown, 0x5c, 0x59},
}

func (o SurfaceOwner) Ground() bool {
	switch o.MaterialOwner() {
	case SurfaceBrown, SurfaceFieldSide, SurfaceFieldTop, SurfaceFallback74, SurfaceFallback7C, SurfaceFallback84:
		return true
	}
	return false
}
func (o SurfaceOwner) Pad() (base SurfaceOwner, written, ok bool) {
	switch o.MaterialOwner() {
	case 0x5b:
		return SurfaceFallback7C, true, true
	case 0x5a:
		return SurfaceFallback7C, false, true
	case 0x5c:
		return SurfaceBrown, true, true
	case 0x59:
		return SurfaceBrown, false, true
	}
	return 0, false, false
}
func (o SurfaceOwner) WithPad(written bool) SurfaceOwner {
	if o >= brownSpan && o < endFieldSpan {
		var layer SurfaceOwner
		switch o &^ 3 {
		case brownSpan:
			layer = brownPadWriteSpan
		case ochre7CSpan:
			layer = ochrePadWriteSpan
		default:
			return 0
		}
		if !written {
			layer += 4
		}
		return layer | (o & 3)
	}
	// A pad over another pad is not evidence for the ground below that later layer.
	switch o {
	case SurfaceFallback7C:
		if written {
			return 0x5b
		}
		return 0x5a
	case SurfaceBrown:
		if written {
			return 0x5c
		}
		return 0x59
	}
	return 0
}
func (o SurfaceOwner) ApprovedGround() bool {
	if o.Ground() {
		return true
	}
	_, _, ok := o.Pad()
	return ok
}

// One source span rule is used by the indexed writer and ownership. Odd tails,
// phase and partial physical subpixels must never be independently approximated.
type screenDoorSpan struct{ first, last, scale, y int }

func sourceScreenDoorSpan(y, l, r, scale int) screenDoorSpan {
	first := l / scale
	if (first+y/scale)&1 != 0 {
		first++
	}
	return screenDoorSpan{first, first + 2*((r/scale-l/scale+1)/2-1), scale, y / scale}
}
func (s screenDoorSpan) writes(x int) bool {
	u := x
	if s.scale == 2 {
		u = x >> 1
	}
	return u >= s.first && u <= s.last && (u+s.y)&1 == 0
}

func (o SurfaceOwner) SourceGround() SurfaceOwner {
	if base, _, ok := o.Pad(); ok {
		return base
	}
	return o.MaterialOwner()
}

type spanOwnership struct {
	feedback *world.TerrainFeedback
	mask     []byte
	material materialBinding
	fallback bool
	sparse   bool // Normal display needs only new84 and B4/pad coverage.
}

func (o *spanOwnership) span(dst []byte, y, l, r int, word uint16) {
	if o == nil || len(o.mask) != len(dst) {
		return
	}
	layout := rasterlayout.FromPixels(len(dst))
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
	owners := o.mask[base+l : base+r+1]
	pixels := dst[base+l : base+r+1]
	if o.material.role != GroundContactPad {
		switch word {
		case 0:
			for i, pixel := range pixels {
				if pixel >= 0x60 {
					owners[i] = owner
				}
			}
		case 0xffff:
			for i, pixel := range pixels {
				if pixel&0xf0 == 0x60 {
					owners[i] = owner
				}
			}
		default:
			// Visit just the written logical cells, clipping both physical ends.
			door := sourceScreenDoorSpan(layout.LogicalY[y], layout.LogicalX[l], layout.LogicalX[r], 1)
			for cell := door.first; cell <= door.last; cell += 2 {
				start := max(layout.X[cell], l)
				end := min(layout.X[cell+1], r+1)
				for i := start; i < end; i++ {
					o.mask[base+i] = owner
				}
			}
		}
		return
	}

	// Transparent pad holes can change ownership even when no colour is written.
	door := sourceScreenDoorSpan(layout.LogicalY[y], layout.LogicalX[l], layout.LogicalX[r], 1)
	for i, pixel := range pixels {
		var write bool
		switch word {
		case 0:
			write = pixel >= 0x60
		case 0xffff:
			write = pixel&0xf0 == 0x60
		default:
			write = door.writes(layout.LogicalX[l+i])
		}
		if layer := SurfaceOwner(owners[i]).WithPad(write); layer != 0 {
			owners[i] = byte(layer)
		} else if write {
			owners[i] = owner
		}
	}
}
func (f *frame) surfaceOwnership(outline bool) *spanOwnership {
	if f.scene.SkyMask == nil {
		return nil
	}
	d := f.material
	if outline {
		d = materialBinding{}
	}
	var feedback *world.TerrainFeedback
	if f.material.owner == byte(SurfaceBrown) && f.styleMask != nil && f.r.Assets.TerrainFeedback.Ready() {
		feedback = f.r.Assets.TerrainFeedback
	}
	return &spanOwnership{feedback: feedback, mask: f.scene.SkyMask, material: d, sparse: f.sparseOwners}
}
