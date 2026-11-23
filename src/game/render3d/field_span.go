package render3d

import (
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
)

// These presentation tokens travel through clipping, pad composition, cockpit
// overlays and display shake in the existing ownership plane. The low two
// bits carry a span result, independent of the canonical indexed raster.
const (
	brownSpan SurfaceOwner = 0xc0 + 4*iota
	brown74Span
	ochre7CSpan
	brownPadWriteSpan
	brownPadHoleSpan
	ochrePadWriteSpan
	ochrePadHoleSpan
	endFieldSpan
)

var fieldOwners = [...]SurfaceOwner{SurfaceBrown, SurfaceFallback74, SurfaceFallback7C, 0x5c, 0x59, 0x5b, 0x5a}

// MaterialOwner removes only the field shade. Other owners, including sky,
// retain their ordinary identities.
func (o SurfaceOwner) MaterialOwner() SurfaceOwner {
	if o >= brownSpan && o < endFieldSpan {
		return fieldOwners[(o-brownSpan)>>2]
	}
	return o
}

func fieldSpanToken(owner byte) SurfaceOwner {
	switch SurfaceOwner(owner) {
	case SurfaceBrown:
		return brownSpan
	case SurfaceFallback74:
		return brown74Span
	case SurfaceFallback7C:
		return ochre7CSpan
	}
	return 0
}

// One sample occupies two logical dots horizontally. As in the jungle leaf,
// each clipped span starts with DL/DH, emits DL for odd length, then feeds the
// previous pair into the next one. Only the low two bits select palette inks.
// The first and last sample may cover only part of their physical footprint.
func fieldFeedbackSpan(row []byte, y, l, r, shift int, token SurfaceOwner, feedback *world.TerrainFeedback) {
	runShift := shift + 1
	first, last := l>>runShift, r>>runShift
	addr := (y >> shift) * Width
	// The three admitted source low bytes B7,77,7F all end in 3.
	a, h := byte(token)|3, byte(token)
	u := first
	put := func(unit int, ink byte) {
		start, end := max(l, unit<<runShift), min(r+1, (unit+1)<<runShift)
		for i := start; i < end; i++ {
			row[i] = ink
		}
	}
	if (last-first+1)&1 != 0 {
		put(u, a)
		u++
	}
	for ; u <= last; u += 2 {
		old := a
		a ^= (h ^ feedback.Low2(addr+u)) & 3
		h = old
		x := u << runShift
		if x < l || ((u+2)<<runShift) > r+1 {
			put(u, a)
			put(u+1, h)
		} else if shift == 0 {
			s := row[x : x+4]
			s[0], s[1], s[2], s[3] = a, a, h, h
		} else {
			s := row[x : x+8]
			s[0], s[1], s[2], s[3] = a, a, a, a
			s[4], s[5], s[6], s[7] = h, h, h, h
		}
	}
}

func mappedFieldSpan(row []byte, y, l, r int, layout *rasterlayout.Layout, token SurfaceOwner, feedback *world.TerrainFeedback) {
	if !layout.Mapped() {
		fieldFeedbackSpan(row, y, l, r, layout.Width/Width-1, token, feedback)
		return
	}
	var logical [Width]byte
	fieldFeedbackSpan(logical[:], layout.LogicalY[y], layout.LogicalX[l], layout.LogicalX[r], 0, token, feedback)
	expandLogicalSpan(row, l, r, layout, logical[:])
}
