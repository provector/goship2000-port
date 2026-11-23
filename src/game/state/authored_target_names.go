package state

import "fmt"

// This optional text-only interpretation freezes the omitted source singleton
// TARGET suffix correction. Absence retains historical compiler-v1/v2 bytes.
const AuthoredTargetNameProfile = "source-authored-single-target-name-v1"

// EnableAuthoredSourceTargetNames is called only by new authoring or explicit
// SAVE. Merely opening an old file or BEGIN never recompiles its realization.
func EnableAuthoredSourceTargetNames(d *AuthoredDocument) bool {
	if d == nil || d.Initial != nil || d.Generated != nil || d.TargetNameProfile != "" {
		return false
	}
	d.TargetNameProfile = AuthoredTargetNameProfile
	d.Compiled = nil
	return true
}

func authoredSingleTargetName(d *AuthoredDocument, g *genericMissionPopulation, p AuthoredPlacement, start, count int) (string, bool, error) {
	if d.TargetNameProfile != AuthoredTargetNameProfile || (p.Role != "primary" && p.Role != "secondary") || (p.Kind != 0 && p.Kind != 4) || count != 1 {
		return "", false, nil
	}
	rows, err := authoredPlacementComponents(g.source, p)
	if err != nil || len(rows) != 1 || start < 0 || start >= len(g.objects) {
		return "", false, fmt.Errorf("single target source identity unavailable")
	}
	ti, err := authoredTemplate(g.source, rows[0])
	if err != nil {
		return "", false, err
	}
	// 1be1/1cad first converts a mandatory formation's leader to7. The
	// following0/4 branch converts other kind>=10h to7 and bypasses name-copy.
	kind := g.source.OBJL[ti*32+10]
	if g.forms[p.Formation].Flags&0x4000 != 0 {
		kind = 7
	}
	if kind >= 0x10 {
		return "", false, nil
	}
	return missionCString(g.objects[start].Raw[15:27]), true, nil
}
