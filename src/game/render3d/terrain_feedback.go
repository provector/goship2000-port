package render3d

import (
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
)

func terrainFeedbackAdmitted(terrain bool, worldID string, opcode byte, word uint16) bool {
	phil := worldID == "PHIL1" || worldID == "PHIL2" || worldID == "PHIL3"
	return terrain && phil && opcode >= 3 && opcode <= 6 && word >= 0x70 && word <= 0x8c && word&3 == 0
}

// Only compact leaf triangles and fruit/crown discs use presentation feedback.
// Trunks, other memory fills and the avionics reference keep their original fill.
func (f *frame) foliageFeedback(p world.Primitive) *world.TerrainFeedback {
	switch f.compactCallback {
	case 1:
		if p.Opcode != 3 || p.Colour != 0x24 {
			return nil
		}
	case 4:
		if p.Opcode != 3 || (p.Colour != 0x24 && p.Colour != 0x26 && p.Colour != 0x28) {
			return nil
		}
	case 5:
		if p.Opcode != 1 || (p.Colour != 0x20 && p.Colour != 0xb0) {
			return nil
		}
	default:
		return nil
	}
	if f.scene.SourceFillReference || f.scene.GraphicsOptions&1 == 0 || f.scene.MaterialCarrier != nil || !f.r.Assets.TerrainFeedback.Ready() {
		return nil
	}
	return f.r.Assets.TerrainFeedback
}

// fillTerrainFeedbackSpan preserves source per-span AX seed, odd leading DL,
// paired XOR feedback and logical320 addressing. The source CS contribution is
// a periodic prepared sequence; mutable DS is explicitly omitted. No state,
// world UV, clock, RNG, float or per-pixel allocation belongs to this material.
func fillTerrainFeedbackSpan(dst []byte, y, l, r int, lo byte, feedback *world.TerrainFeedback) {
	if len(dst) == Width*Height*4 {
		first, last := l/2, r/2
		a, h := lo, lo^7
		u := first
		base := y * (Width * 2)
		if (last-first+1)&1 != 0 {
			putTerrainLogical(dst, base, l, r, u, a)
			u++
		}
		for u <= last {
			q := (h ^ feedback.Low2((y/2)*Width+u)) & 3
			a ^= q
			h = q ^ a
			putTerrainLogical(dst, base, l, r, u, a)
			putTerrainLogical(dst, base, l, r, u+1, h)
			u += 2
		}
		return
	}
	if len(dst) > Width*Height*4 {
		layout := rasterlayout.FromPixels(len(dst))
		var logical [Width]byte
		terrainLogicalSpan(logical[:], layout.LogicalY[y], layout.LogicalX[l], layout.LogicalX[r], lo, feedback)
		expandLogicalSpan(dst[y*layout.Width:], l, r, layout, logical[:])
		return
	}
	terrainLogicalSpan(dst[y*Width:], y, l, r, lo, feedback)
}
func terrainLogicalSpan(row []byte, y, l, r int, lo byte, feedback *world.TerrainFeedback) {
	x := l
	a, h := lo, lo^7
	base := y * Width
	if (r-l+1)&1 != 0 {
		row[x] = a
		x++
	}
	for x <= r {
		at := base + x
		q := (h ^ feedback.Low2(at)) & 3
		a ^= q
		h = q ^ a
		row[x], row[x+1] = a, h
		x += 2
	}
}

func putTerrainLogical(dst []byte, base, l, r, logicalX int, colour byte) {
	x := logicalX * 2
	if x >= l && x <= r {
		dst[base+x] = colour
	}
	if x+1 >= l && x+1 <= r {
		dst[base+x+1] = colour
	}
}
