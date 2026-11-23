package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

// Train is an authored constructor interpretation, not a changed simulator.
// Source20F4:646E copies the just-tested terminal40 row. 658D saves the old
// index before6590 increments it; the earlier 'excludes40' reading was wrong.
const AuthoredTrainProfile = "source-editor-train-preset-v1"
const AuthoredTrainRouteProfile = AuthoredTrainProfile

func authoredTrainPaths(w *assetdata.MissionWorld) ([2][]missionPath, error) {
	var out [2][]missionPath
	if w == nil || len(w.PATH)%9 != 0 {
		return out, fmt.Errorf("Train source PATH has a partial row")
	}
	for at, n := 0, 0; at < len(w.PATH)/9 && n < 2; at++ {
		if w.PATH[at*9+8] != 8 {
			continue
		}
		end := at
		for end < len(w.PATH)/9 && w.PATH[end*9+8] != 0x40 {
			end++
		}
		if end == len(w.PATH)/9 {
			return out, fmt.Errorf("Train source PATH lacks its copied terminal40")
		}
		for i := at; i <= end; i++ {
			r := w.PATH[i*9:]
			out[n] = append(out[n], missionPath{Point: MissionPoint{X: int32(binary.LittleEndian.Uint32(r)), Y: int32(binary.LittleEndian.Uint32(r[4:]))}, Flag: r[8]})
		}
		n++
		at = end
	}
	if len(out[0]) < 2 || len(out[1]) < 2 {
		return out, fmt.Errorf("Train needs both complete source preset paths")
	}
	return out, nil
}

func authoredTrainAnchor(p MissionPoint) [2]int {
	return [2]int{MissionI16(int(p.X/341) - 1), MissionI16(int(p.Y/-455) + 1153)}
}

// 20F4:6640: signed-word Manhattan comparison; equality selects the second
// original start. No terrain planner, host time or compilation RNG is involved.
func authoredTrainShadow(w *assetdata.MissionWorld, x, y int) ([2]int, error) {
	paths, err := authoredTrainPaths(w)
	if err != nil {
		return [2]int{}, err
	}
	a, b := authoredTrainAnchor(paths[0][0].Point), authoredTrainAnchor(paths[1][0].Point)
	distance := func(p [2]int) int {
		return MissionI16(missionAbs(MissionI16(x-p[0])) + missionAbs(MissionI16(y-p[1])))
	}
	if distance(b) <= distance(a) {
		return b, nil
	}
	return a, nil
}

// 646E chooses first only on the complete signed source coordinate equality;
// otherwise second. Interactive Fill does not run6640 and retains that rule.
func authoredTrainRows(w *assetdata.MissionWorld, p AuthoredPlacement) ([]missionPath, error) {
	if p.TrainProfile != AuthoredTrainProfile || p.Formation < 0 || p.Formation >= len(w.FMHD)/39 || binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x200 == 0 {
		return nil, fmt.Errorf("Train source constructor interpretation is absent or mismatched")
	}
	if p.Role != "defender" && p.Role != "primary" && p.Role != "secondary" {
		return nil, fmt.Errorf("Train is not a base or service site")
	}
	paths, err := authoredTrainPaths(w)
	if err != nil {
		return nil, err
	}
	a := authoredTrainAnchor(paths[0][0].Point)
	if p.X == a[0] && p.Y == a[1] {
		return paths[0], nil
	}
	return paths[1], nil
}

func authoredTrainRoute(w *assetdata.MissionWorld, p AuthoredPlacement) (*AuthoredRoute, error) {
	rows, err := authoredTrainRows(w, p)
	if err != nil {
		return nil, err
	}
	last := rows[len(rows)-1].Point
	// 65A8..65FC ->1CF6 writes the copied terminal's source coordinates into
	// C368/C36A. Interactive kind2 Fill reads these, not the start marker.
	return &AuthoredRoute{Profile: AuthoredTrainRouteProfile, X: MissionI16(int(last.X/341) - 1), Y: MissionI16(int(last.Y/-455) + 1151)}, nil
}
