package menus

import (
	"gunship2000-assets/game/render3d"
	"image"
	"image/color"
)

// BrownTerrainPresentationPolicy replaces only the identified EURO6 terrain
// memory-fallback hatch after canonical feedback. It is a deliberate native
// approximation, not a claim about original live CS/DS material bytes.
const BrownTerrainPresentationPolicy = "euro6-brown-terrain-family-grain-after-canonical-v1"

func blendBrownTerrainPresentation(dst *image.RGBA, indexed, mask []byte, palette color.Palette, blocked image.Rectangle) {
	if dst == nil || len(indexed) != len(mask) || len(palette) != 256 || len(indexed) != dst.Rect.Dx()*dst.Rect.Dy() {
		return
	}
	var red, green, blue uint32
	var first [3]uint32
	uniform := true
	for i := 0xb4; i <= 0xb7; i++ {
		r, g, b, _ := palette[i].RGBA()
		if i == 0xb4 {
			first = [3]uint32{r, g, b}
		} else if first != ([3]uint32{r, g, b}) {
			uniform = false
		}
		red += r >> 8
		green += g >> 8
		blue += b >> 8
	}
	clamp := func(v int) byte { return byte(max(0, min(255, v))) }
	scale := max(1, dst.Rect.Dx()/320)
	for at, owner := range mask {
		if render3d.SurfaceOwner(owner).MaterialOwner() != render3d.SurfaceBrown || indexed[at] < 0xb4 || indexed[at] > 0xb7 {
			continue // Models, weather remaps and UI never gain ownership by colour.
		}
		x, y := at%dst.Rect.Dx(), at/dst.Rect.Dx()
		if image.Pt(x, y).In(blocked) {
			continue
		}
		// Fine logical-pixel grain is stable at either geometry resolution. It
		// preserves the active four-colour mean/hue without the large4x4 hatch.
		hash := uint32(x/scale)*0x9e3779b1 ^ uint32(y/scale)*0x85ebca6b
		hash ^= hash >> 16
		grain := int(hash%5) - 2
		if uniform {
			grain = 0
		}
		dst.SetRGBA(x, y, color.RGBA{clamp(int((red+2)/4) + grain), clamp(int((green+2)/4) + grain), clamp(int((blue+2)/4) + grain), 255})
	}
}

// EuroGroundPresentationPolicy suppresses only the deterministic coarse hatch
// standing in for undefined source CS/DS texture memory. Genuine patterned
// colour words, supplied source-memory carriers, models and sky are excluded by
// geometry ownership, not guessed from the final colour or screenshot filename.
const EuroGroundPresentationPolicy = "euro-ground-fallback-family-mean-v1"

func groundFamily(owner byte) (byte, bool) {
	owner = byte(render3d.SurfaceOwner(owner).MaterialOwner())
	if owner == 0xb4 {
		return 0xb4, true
	}
	if owner == byte(render3d.SurfaceFallback84) {
		return 0x84, true
	}
	if owner == 0x49 || owner == 0x5d || owner == 0x5f {
		return (owner & 0x3f) << 2, true
	}
	return 0, false
}
func blendEuroGroundPresentation(dst *image.RGBA, indexed, mask []byte, palette color.Palette, blocked image.Rectangle) {
	if dst == nil || len(indexed) != len(mask) || len(palette) != 256 || len(indexed) != dst.Rect.Dx()*dst.Rect.Dy() {
		return
	}
	var means [64]color.RGBA
	for family := 0; family < 256; family += 4 {
		var sums [3]uint32
		for i := family; i < family+4; i++ {
			r, g, b, _ := palette[i].RGBA()
			sums[0] += r >> 8
			sums[1] += g >> 8
			sums[2] += b >> 8
		}
		means[family>>2] = color.RGBA{byte((sums[0] + 2) / 4), byte((sums[1] + 2) / 4), byte((sums[2] + 2) / 4), 255}
	}
	for at, owner := range mask {
		family, ok := groundFamily(owner)
		if !ok || family == 0xb4 || indexed[at]&0xfc != family {
			continue
		}
		x, y := at%dst.Rect.Dx(), at/dst.Rect.Dx()
		if !image.Pt(x, y).In(blocked) {
			dst.SetRGBA(x, y, means[family>>2])
		}
	}
}

// EuroFieldPresentationPolicy is a named approximation for the reported EURO7
// field's explicit89/8C screen-parity fill. It is separate from memory fallback
// and does not treat another same-colour surface, edge or model as terrain.
const EuroFieldPresentationPolicy = "euro7-field-898c-pair-mean-after-canonical-v1"

func blendEuroFieldPresentation(dst *image.RGBA, indexed, mask []byte, palette color.Palette, blocked image.Rectangle) {
	if dst == nil || len(indexed) != len(mask) || len(indexed) != dst.Rect.Dx()*dst.Rect.Dy() || len(palette) != 256 {
		return
	}
	r0, g0, b0, _ := palette[0x89].RGBA()
	r1, g1, b1, _ := palette[0x8c].RGBA()
	mean := color.RGBA{byte(((r0 >> 8) + (r1 >> 8) + 1) / 2), byte(((g0 >> 8) + (g1 >> 8) + 1) / 2), byte(((b0 >> 8) + (b1 >> 8) + 1) / 2), 255}
	for at, owner := range mask {
		if owner != 0x58 || indexed[at] != 0x89 && indexed[at] != 0x8c {
			continue
		}
		x, y := at%dst.Rect.Dx(), at/dst.Rect.Dx()
		if !image.Pt(x, y).In(blocked) {
			dst.SetRGBA(x, y, mean)
		}
	}
}

// EuroPadPresentationPolicy resolves only the identified screen-door pad over
// proved7C terrain. Source yellow0E markings, other pad backgrounds and all later
// scenery/HUD writes keep their independent ownership.
const EuroPadPresentationPolicy = "euro7-pad131-ec00-over-7c-half-mean-after-canonical-v1"

func blendEuroPadPresentation(dst *image.RGBA, indexed, mask []byte, palette color.Palette, blocked image.Rectangle) {
	filtered := make([]byte, len(mask))
	for i, v := range mask {
		if _, _, ok := render3d.SurfaceOwner(v).Pad(); ok {
			filtered[i] = v
		}
	}
	render3d.ResolveGroundPresentation(dst, indexed, filtered, palette, blocked, nil)
}
