package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

// Source6040 shadow then1998/1A00 chooses the rescue formation from reached
// terrain: the first10|0400|2000 entry for water; the second10 entry for land.
// The player does not receive an invented FMHD choice between these carriers.
func authoredRescueShadow(data *assetdata.MissionGenerationData, d *AuthoredDocument, pop *missionPopulation, role string, x, y int) (AuthoredPlacement, error) {
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return AuthoredPlacement{}, err
	}
	if x < 0 || x >= 1536 || y < 0 || y >= 1152 {
		return AuthoredPlacement{}, fmt.Errorf("rescue cursor outside source map")
	}
	flags := uint32(0)
	if w.LAND[w.Grid[x/24+64*(y/18)]&63]&2 != 0 {
		flags = 0x200000
	}
	found, ok := pop.spiral(x/24, 63-y/18, flags)
	if !ok {
		return AuthoredPlacement{}, fmt.Errorf("no source-eligible rescue shadow")
	}
	sx, sy := 24*found[0]+12, 18*(63-found[1])+9
	terrain := w.Grid[sx/24+64*(sy/18)] & 63
	water := w.LAND[terrain]&2 != 0
	first, fi := -1, -1
	for i := 0; i < len(w.FMHD)/39; i++ {
		f := binary.LittleEndian.Uint32(w.FMHD[i*39+35:])
		if f&0x10 == 0 {
			continue
		}
		if water {
			if f&0x2400 == 0x2400 {
				fi = i
				break
			}
		} else {
			if first >= 0 {
				fi = i
				break
			}
			first = i
		}
	}
	if fi < 0 {
		return AuthoredPlacement{}, fmt.Errorf("source rescue formation for reached terrain is absent")
	}
	p, err := authoredPlacement(w, fi, role, 1)
	if err != nil {
		return p, err
	}
	p.SelectionProfile = AuthoredSelectionProfile
	p.X, p.Y, p.Terrain = sx, sy, terrain
	return p, nil
}
