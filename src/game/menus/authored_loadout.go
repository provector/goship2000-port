package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

// Native documents may retain actual accepted preflight rows. This is an
// explicit native capability; original SIM copied selectors, not full SHIP.
// Recompute environment-dependent capacities under the current authored world
// and validate source fits/quantities before ordinary preflight can accept them.
func authoredLoadout(a *assetdata.MissionAssets, m *gamestate.MissionData, saved [][36]byte, rank gamestate.Rank) ([][36]byte, error) {
	if len(saved) != 5 && !(m.SoloBuilder && len(saved) == 1) {
		return nil, fmt.Errorf("authored loadout requires five actual aircraft")
	}
	rows := append([][36]byte(nil), saved...)
	for i := range rows {
		r := rows[i][:]
		id := int(r[0])
		sh := a.AircraftDefinitions[id]
		if id > 7 || len(sh) != 252 || r[1] != sh[1] || r[2] != sh[3] || int(int8(sh[5])) > rank.Raw() {
			return nil, fmt.Errorf("aircraft%d is unavailable to the current pilot", id)
		}
		fresh, e := gamestate.InitializeReadinessRow(a, m, id)
		if e != nil {
			return nil, e
		}
		for _, at := range []int{23, 27} {
			gamestate.RowPut(r, at, int(gamestate.RowWord(fresh[:], at)))
		}
		for station := 0; station < 3; station++ {
			count := gamestate.RowWord(r, 12+2*station)
			if count < 0 {
				return nil, fmt.Errorf("negative authored weapon quantity")
			}
			if count == 0 {
				continue
			}
			system := int(int8(r[3+station]))
			def := a.WeaponDefinitions[system]
			if len(def) != 214 {
				return nil, fmt.Errorf("authored system%d unavailable", system)
			}
			found := false
			for _, f := range a.Preflight.Ships[id].Fits {
				if f.Station == station && f.System == system {
					limit := int(int8(sh[9])) * f.Quantity
					if int(gamestate.RowWord(def, 4))&0x4000 != 0 {
						limit = 1
					}
					if int(count) <= limit {
						found = true
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("authored system%d/station%d does not fit aircraft%d", system, station, id)
			}
		}
		if gamestate.RowWord(r, 29) < 0 || gamestate.RowWord(r, 29) > gamestate.RowWord(r, 27) || gamestate.RowWord(r, 31) < 0 || gamestate.RowWord(r, 31) > 60 || gamestate.RowWord(r, 33) < 0 || gamestate.RowWord(r, 33) > 60 {
			return nil, fmt.Errorf("authored stores outside source limits")
		}
		if def := a.WeaponDefinitions[int(int8(r[2]))]; len(def) == 214 && (gamestate.RowWord(r, 21) < 0 || gamestate.RowWord(r, 21) > gamestate.RowWord(def, 2)) {
			return nil, fmt.Errorf("authored cannon ammunition outside source limit")
		}
		if e = gamestate.RecomputeReadinessRow(a, sh, r); e != nil {
			return nil, e
		}
	}
	if e := gamestate.PreflightReadiness(rows); e != nil {
		return nil, e
	}
	return rows, nil
}
