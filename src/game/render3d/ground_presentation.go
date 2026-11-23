package render3d

import (
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
	"image"
	"image/color"
)

// ResolveGroundPresentation is shared by all normal scene displays. It reads
// the active palette and typed source ownership, never infers a writer from ink.
// It cannot change the indexed reference, geometry, RNG or simulation state.
func ResolveGroundPresentation(dst *image.RGBA, indexed, mask []byte, palette color.Palette, blocked image.Rectangle, streaks *world.TerrainFeedback) {
	if dst == nil || len(mask) != len(indexed) || len(mask) != dst.Rect.Dx()*dst.Rect.Dy() || len(palette) != 256 {
		return
	}
	rgb := func(i int) color.RGBA {
		r, g, b, _ := palette[i].RGBA()
		return color.RGBA{byte(r >> 8), byte(g >> 8), byte(b >> 8), 255}
	}
	mean := func(a, z int) color.RGBA {
		var sums [3]uint32
		for i := a; i <= z; i++ {
			r, g, b, _ := palette[i].RGBA()
			sums[0] += r >> 8
			sums[1] += g >> 8
			sums[2] += b >> 8
		}
		n := uint32(z - a + 1)
		return color.RGBA{byte((sums[0] + n/2) / n), byte((sums[1] + n/2) / n), byte((sums[2] + n/2) / n), 255}
	}
	// The ownership byte is already a compact discriminant. A map lookup here
	// used to hash every destination (including sky and ordinary objects).
	// Four palette phases serve the prepared low-two-bit pattern. Uniform
	// materials have the same colour in every phase. Nil selects checker parity.
	var styles [256][4]color.RGBA
	horizontal := streaks.Ready()
	flat := func(owner SurfaceOwner, c color.RGBA) { styles[owner] = [4]color.RGBA{c, c, c, c} }
	flat(SurfaceFieldSide, mean(0x24, 0x27))
	flat(SurfaceFallback84, mean(0x84, 0x87))
	a, z := rgb(0x89), rgb(0x8c)
	blend := func(a, b color.RGBA) color.RGBA {
		return color.RGBA{byte((uint16(a.R) + uint16(b.R) + 1) / 2), byte((uint16(a.G) + uint16(b.G) + 1) / 2), byte((uint16(a.B) + uint16(b.B) + 1) / 2), 255}
	}
	flat(SurfaceFieldTop, blend(a, z))
	// Each admitted brown/ochre missing-carrier field uses its source bit-clear
	// pair, lo and lo XOR 7. Other material branches keep their own recipes.
	for _, f := range [...]struct {
		owner SurfaceOwner
		lo    byte
	}{{SurfaceBrown, 0xb7}, {SurfaceFallback74, 0x77}, {SurfaceFallback7C, 0x7f}} {
		if horizontal {
			for shade := range styles[f.owner] {
				styles[f.owner][shade] = rgb(int(f.lo&0xfc) + shade)
			}
		} else {
			styles[f.owner][0], styles[f.owner][1] = rgb(int(f.lo)), rgb(int(f.lo^7))
		}
	}
	grey := rgb(0xec)
	for _, p := range padLayers {
		colours := styles[p.base]
		for i, c := range colours {
			colours[i] = blend(c, grey)
		}
		styles[p.write], styles[p.hole] = colours, colours
	}
	// Resolve shade-bearing tokens once per palette. Off uses the shade
	// emitted by the clipped field span; On still selects logical parity.
	for token := brownSpan; token < endFieldSpan; token++ {
		base := token.MaterialOwner()
		if horizontal {
			flat(token, styles[base][token&3])
		} else {
			styles[token] = styles[base]
		}
	}
	width := dst.Rect.Dx()
	layout := rasterlayout.FromPixels(len(mask))
	high := layout != nil && layout.Mapped()
	shift := 0
	if width == 2*Width {
		shift = 1
	}
	for y := 0; y < dst.Rect.Dy(); y++ {
		start := y * width
		row := mask[start : start+width]
		logicalY := y >> shift
		if high {
			logicalY = layout.LogicalY[y]
		}
		for x, token := range row {
			if token == 0 || token == 1 {
				continue
			}
			logicalX := x >> shift
			if high {
				logicalX = layout.LogicalX[x]
			}
			phase := checkerParity(logicalX, logicalY)
			c := styles[token][phase]
			if c.A == 0 || !allowedGroundInk[token].admits(indexed[start+x]) || image.Pt(x, y).In(blocked) {
				continue
			}
			i := y*dst.Stride + 4*x
			copy(dst.Pix[i:i+4], []byte{c.R, c.G, c.B, c.A})
		}
	}
}

// Index validation rejects stale external masks. Resolve each compact token
// once so the inner loop does not decode the material and pad layer again.
type groundInkRange struct{ first, last, extra byte }

func (r groundInkRange) admits(index byte) bool {
	return (index >= r.first && index <= r.last) || index == r.extra
}

var allowedGroundInk = func() (rules [256]groundInkRange) {
	for i := range rules {
		owner := SurfaceOwner(i)
		var first, last, extra byte
		switch owner.SourceGround() {
		case SurfaceFieldTop:
			first, last, extra = 0x89, 0x89, 0x8c
		case SurfaceBrown:
			first, last, extra = 0xb4, 0xb7, 0xb4
		case SurfaceFieldSide:
			first, last, extra = 0x24, 0x27, 0x24
		case SurfaceFallback74:
			first, last, extra = 0x74, 0x77, 0x74
		case SurfaceFallback7C:
			first, last, extra = 0x7c, 0x7f, 0x7c
		case SurfaceFallback84:
			first, last, extra = 0x84, 0x87, 0x84
		default:
			continue // No style is bound.
		}
		if _, _, pad := owner.Pad(); pad {
			extra = 0xec
		}
		rules[i] = groundInkRange{first, last, extra}
	}
	return
}()
