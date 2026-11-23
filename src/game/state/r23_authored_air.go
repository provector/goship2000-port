package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

// Only newly selected enemy air/submarine descriptors use this interpretation.
// Existing P5 selections and saved compiled realizations retain their meaning.
const AuthoredSelectionProfileR23 = "source-editor-enemy-selection-v4"

func authoredP5Selection(profile string) bool {
	return profile == AuthoredSelectionProfileP5 || profile == AuthoredSelectionProfileR23
}

func authoredEnemySelection(w *assetdata.MissionWorld, p AuthoredPlacement) bool {
	if p.Formation < 0 || p.Formation >= len(w.FMHD)/39 || p.Template < 0 || p.Template >= len(w.OBJL)/32 {
		return false
	}
	kind := w.OBJL[p.Template*32+10]
	return kind == 1 && binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x100000 != 0 || kind == 0x0a
}

// 20F4:68B5 installs low/high margins1/4 for619C. Scan order, duplicate
// edge candidates and strict ties match the existing Deep Strike2/8 owner.
func authoredEnemyEdgeShadow(w *assetdata.MissionWorld, d *AuthoredDocument, x, y int) ([2]int, error) {
	if x < -9 || x >= 1536 || y < -9 || y >= 1152 {
		return [2]int{}, fmt.Errorf("enemy ingress cursor outside source map")
	}
	cx, cy := int(int16(x))/24, int(int16(y))/18
	best, bx, by := 9999, -1, -1
	occupied := map[[2]int]bool{}
	for _, p := range d.Placements {
		occupied[[2]int{p.X / 24, p.Y / 18}] = true
	}
	for n := 1; n <= 62; n++ {
		t := max(4, min(59, n))
		for _, p := range [][2]int{{1, t}, {62, t}, {t, 1}, {t, 62}} {
			cell := int(w.Grid[p[0]+64*p[1]])
			distance := absEntry(cx-p[0]) + absEntry(cy-p[1])
			if distance >= best || cell&0x80 != 0 || occupied[p] || cell >= len(w.LAND) || w.LAND[cell]&0x40 != 0 {
				continue
			}
			best, bx, by = distance, p[0], p[1]
		}
	}
	if bx < 0 {
		return [2]int{}, fmt.Errorf("no source-eligible enemy ingress edge")
	}
	return [2]int{24*bx + 12, 18*by + 9}, nil
}
