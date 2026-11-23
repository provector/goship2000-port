package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredEntryProfile = "source-editor-deep-strike-v1"

// Source E28C2 owns an edge entry, not a physical base OBJECT. The placement
// keeps its source direction descriptor; the accepted realization freezes its
// inactive base TARGET and player position independently.
type AuthoredDeepStrikeEntry struct {
	Profile string `json:"profile"`
}

// Source20F4:68ED..690B sets low/high margins2/8, then619C scans the four
// edges in this exact strict-distance order. Occupied80 and LAND40 reject.
func authoredEntryShadow(w *assetdata.MissionWorld, d *AuthoredDocument, x, y int) (AuthoredPlacement, error) {
	if x < -9 || x >= 1536 || y < -9 || y >= 1152 {
		return AuthoredPlacement{}, fmt.Errorf("entry cursor outside source map")
	}
	cx, cy := int(int16(x))/24, int(int16(y))/18
	best, bx, by, side := 9999, -1, -1, 0
	occupied := map[[2]int]bool{}
	for _, p := range d.Placements {
		if p.Role != "base" {
			occupied[[2]int{p.X / 24, p.Y / 18}] = true
		}
	}
	for n := 2; n <= 61; n++ {
		t := max(8, min(55, n))
		for _, p := range [][3]int{{2, t, 1}, {61, t, 2}, {t, 2, 4}, {t, 61, 3}} {
			cell := int(w.Grid[p[0]+64*p[1]])
			distance := absEntry(cx-p[0]) + absEntry(cy-p[1])
			if distance >= best || cell&0x80 != 0 || occupied[[2]int{p[0], p[1]}] || cell >= len(w.LAND) || w.LAND[cell]&0x40 != 0 {
				continue
			}
			best, bx, by, side = distance, p[0], p[1], p[2]
		}
	}
	if side == 0 {
		return AuthoredPlacement{}, fmt.Errorf("no source-eligible Deep Strike edge")
	}
	fi := -1
	for i := 0; i < len(w.FMHD)/39; i++ {
		if binary.LittleEndian.Uint32(w.FMHD[i*39+35:])&0x800000 != 0 {
			side--
			if side == 0 {
				fi = i
				break
			}
		}
	}
	if fi < 0 {
		return AuthoredPlacement{}, fmt.Errorf("Deep Strike edge has no source directional descriptor")
	}
	ci, err := authoredFirst(w, fi)
	if err != nil {
		return AuthoredPlacement{}, err
	}
	ti, _ := authoredTemplate(w, ci)
	return AuthoredPlacement{Role: "base", Formation: fi, Component: ci, Template: ti, X: 24*bx + 12, Y: 18*by + 9, Terrain: w.Grid[bx+64*by] & 63, SelectionProfile: AuthoredSelectionProfile, Entry: &AuthoredDeepStrikeEntry{Profile: AuthoredEntryProfile}}, nil
}
func absEntry(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func validateAuthoredEntry(data *assetdata.MissionGenerationData, d *AuthoredDocument, p AuthoredPlacement) error {
	if p.Entry == nil {
		return nil
	}
	if p.Entry.Profile != AuthoredEntryProfile || p.Role != "base" || d.Initial != nil || (d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5) {
		return fmt.Errorf("unsupported source Deep Strike owner")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	q, err := authoredEntryShadow(w, d, p.X, p.Y)
	if err != nil {
		return err
	}
	if q.Formation != p.Formation || q.Component != p.Component || q.Template != p.Template || q.X != p.X || q.Y != p.Y || q.Terrain != p.Terrain {
		return fmt.Errorf("Deep Strike source edge/provenance differs")
	}
	return nil
}

// Source1ABF:0006 chooses the inactive TARGET formation independently of the
// editor direction descriptor. Its three signed72-margin comparisons are kept
// literally; it does not find or fabricate a landing base.
func authoredEntryTargetFormation(w *assetdata.MissionWorld, p AuthoredPlacement) (int, error) {
	fi := 0
	for fi < len(w.FMHD)/39 && binary.LittleEndian.Uint32(w.FMHD[fi*39+35:])&0x800000 != 0 {
		fi++
	}
	if 1536-72 < p.X {
		fi++
	}
	if 1152-72 < p.Y {
		fi++
	}
	if 72 > p.X {
		fi++
	}
	if fi < 0 || fi >= len(w.FMHD)/39 {
		return 0, fmt.Errorf("entry TARGET formation outside source table")
	}
	return fi, nil
}

func authoredEntryFromMission(data *assetdata.MissionGenerationData, m *MissionData) (*AuthoredPlacement, error) {
	if m == nil || m.Inputs.MissionType != MissionSimulated || m.Conditions.Flags&0x800 == 0 {
		return nil, nil
	}
	base, ok := m.Objectives["base"]
	if !ok || base.ObjectIndex != 0 || base.RawKind != 0 || base.DisplayKind != 0 || base.Count != 0 || m.Targets == nil || binary.LittleEndian.Uint16(m.Targets.Records[0][32:]) != 0 || len(m.Objects) != 80 || base.Point != m.Objects[0].Point() {
		return nil, fmt.Errorf("Deep Strike Export lacks complete source entry owners")
	}
	p := base.Point
	if (p.X+1)%341 != 0 || (0x7ffff-p.Y)%455 != 0 {
		return nil, fmt.Errorf("Deep Strike Export entry is not a source coordinate")
	}
	x, y := int((p.X+1)/341), int((0x7ffff-p.Y)/455)
	w, err := AuthoredWorld(data, m.World)
	if err != nil {
		return nil, err
	}
	q, err := authoredEntryShadow(w, &AuthoredDocument{}, x, y)
	if err != nil {
		return nil, err
	}
	if q.X != x || q.Y != y {
		return nil, fmt.Errorf("Deep Strike Export is not an eligible source edge")
	}
	q.ID, err = authoredID()
	if err != nil {
		return nil, err
	}
	return &q, nil
}
func validateRetainedAuthoredEntry(data *assetdata.MissionGenerationData, d *AuthoredDocument, c *AuthoredRealization) error {
	if d.Compiler != AuthoredCompilerP5 && d.Compiler != AuthoredCompilerSupportP5 {
		return fmt.Errorf("retained entry requires source compiler v2")
	}
	if c.Entry == nil {
		return fmt.Errorf("retained Deep Strike entry missing")
	}
	p := *c.Entry
	if !authoredValidID(p.ID) || p.Role != "base" || p.Entry == nil || c.Conditions.Flags&0x800 == 0 {
		return fmt.Errorf("retained Deep Strike source carrier differs")
	}
	q := *d
	q.Initial = nil
	q.Compiler = AuthoredCompilerP5
	q.Placements = nil
	if err := validateAuthoredEntry(data, &q, p); err != nil {
		return err
	}
	point := authoredSearchWorld(AuthoredSearchPoint{X: p.X, Y: p.Y})
	base, ok := c.Objectives["base"]
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	fi, err := authoredEntryTargetFormation(w, p)
	if err != nil {
		return err
	}
	if !ok || base.Formation != fi || base.ObjectIndex != 0 || base.RawKind != 0 || base.DisplayKind != 0 || base.Count != 0 || base.Point != point || base.OriginalPoint != point || c.Objects[0].Point() != point {
		return fmt.Errorf("retained inactive TARGET/player entry differs")
	}
	r := c.Targets.Records[0]
	if binary.LittleEndian.Uint16(r[32:]) != 0 {
		return fmt.Errorf("retained entry became physical base")
	}
	for _, off := range []int{34, 42} {
		if int32(binary.LittleEndian.Uint32(r[off:])) != point.X || int32(binary.LittleEndian.Uint32(r[off+4:])) != point.Y {
			return fmt.Errorf("retained entry/return anchor differs")
		}
	}
	return nil
}
