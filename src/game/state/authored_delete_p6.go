package state

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const AuthoredGoalCopyProfile = "source-detached-editor-goal-v1"

func authoredDetachedGoal(p AuthoredPlacement) bool {
	return p.GoalCopyProfile == AuthoredGoalCopyProfile && p.Reference == "" && (p.Role == "primary" || p.Role == "secondary") && (p.Kind == 0 || p.Kind == 2 || p.Kind == 4 || p.Kind == 6)
}
func authoredGoalHasCarrier(p AuthoredPlacement) bool {
	return p.Reference != "" || authoredDetachedGoal(p)
}

func authoredOrdinaryIndexes(d *AuthoredDocument) (out []int) {
	for i, p := range d.Placements {
		if p.Role == "defender" {
			out = append(out, i)
		}
	}
	return
}

// AuthoredSourceOrdinary excludes retained native provenance carriers whose
// actual source row was transferred to C4F0/C504 by20F4:1AE8..1B35. Those
// records remain in the document to restore identity on goal reassignment; they
// are not C018 ordinary rows for last-N deletion, Fill boundaries or75 capacity.
func AuthoredSourceOrdinary(d *AuthoredDocument, id string) bool {
	if d == nil {
		return false
	}
	for _, p := range d.Placements {
		if p.Reference == id {
			return false
		}
	}
	for _, p := range d.Placements {
		if p.ID == id {
			return p.Role == "defender"
		}
	}
	return false
}
func authoredSourceOrdinaryIndexes(d *AuthoredDocument) (out []int) {
	refs := map[string]bool{}
	for _, p := range d.Placements {
		if p.Reference != "" {
			refs[p.Reference] = true
		}
	}
	for i, p := range d.Placements {
		if p.Role == "defender" && !refs[p.ID] {
			out = append(out, i)
		}
	}
	return
}

// GS20F4:18C8 keeps the special role records independent, credits their
// component costs, then swaps the last ordinary11-byte descriptor into the
// vacated index (1934..1972). Stable native references preserve those copies by
// detaching a deleted ordinary carrier, never deleting or retargeting its goal.
func authoredDeleteOrdinary(w *assetdata.MissionWorld, d *AuthoredDocument, ordinary int) (bool, error) {
	return authoredDeleteFromIndexes(w, d, authoredOrdinaryIndexes(d), ordinary)
}
func authoredDeleteSourceOrdinary(w *assetdata.MissionWorld, d *AuthoredDocument, ordinary int) (bool, error) {
	return authoredDeleteFromIndexes(w, d, authoredSourceOrdinaryIndexes(d), ordinary)
}
func authoredDeleteFromIndexes(w *assetdata.MissionWorld, d *AuthoredDocument, indices []int, ordinary int) (bool, error) {
	if ordinary < 0 || ordinary >= len(indices) {
		return false, nil
	}
	i := indices[ordinary]
	p := d.Placements[i]
	if p.Formation < 0 || p.Formation >= len(w.FMHD)/39 {
		return false, fmt.Errorf("source delete formation unavailable")
	}
	if p.Builtin || binary.LittleEndian.Uint32(w.FMHD[p.Formation*39+35:])&0x4000 != 0 {
		return false, nil
	}
	for j := range d.Placements {
		if d.Placements[j].Reference == p.ID {
			goal := &d.Placements[j]
			if (goal.Role != "primary" && goal.Role != "secondary") || (goal.Kind != 0 && goal.Kind != 2 && goal.Kind != 4 && goal.Kind != 6) {
				return false, fmt.Errorf("unsupported source goal-copy owner; draft retained")
			}
			goal.Reference = ""
			goal.GoalCopyProfile = AuthoredGoalCopyProfile
		}
	}
	last := indices[len(indices)-1]
	d.Placements[i] = d.Placements[last]
	d.Placements = append(d.Placements[:last], d.Placements[last+1:]...)
	return true, nil
}

func RemoveAuthoredSource(data *assetdata.MissionGenerationData, d *AuthoredDocument, id string) error {
	if d.Initial != nil {
		return fmt.Errorf("retained film geometry requires a verified inverse compiler")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return err
	}
	for n, i := range authoredOrdinaryIndexes(q) {
		if q.Placements[i].ID != id {
			continue
		}
		changed, err := authoredDeleteOrdinary(w, q, n)
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("built-in formation cannot be deleted")
		}
		q.Compiled = nil
		*d = *q
		return nil
	}
	return fmt.Errorf("ordinary placement not found")
}

// RemoveAuthoredSourceP7 is the current editor source-row operation. The older
// direct helper remains pinned for historical diagnostic callers; no film or
// saved realization invokes either authoring mutation while being viewed.
func RemoveAuthoredSourceP7(data *assetdata.MissionGenerationData, d *AuthoredDocument, id string) error {
	if d.Initial != nil {
		return fmt.Errorf("retained film geometry requires a verified inverse compiler")
	}
	if !AuthoredSourceOrdinary(d, id) {
		return fmt.Errorf("this source row belongs to an objective; edit the objective instead")
	}
	w, err := AuthoredWorld(data, d.World)
	if err != nil {
		return err
	}
	q, err := CloneAuthored(d)
	if err != nil {
		return err
	}
	for n, i := range authoredSourceOrdinaryIndexes(q) {
		if q.Placements[i].ID == id {
			changed, e := authoredDeleteSourceOrdinary(w, q, n)
			if e != nil {
				return e
			}
			if !changed {
				return fmt.Errorf("built-in formation cannot be deleted")
			}
			q.Compiled = nil
			*d = *q
			return nil
		}
	}
	return fmt.Errorf("source ordinary placement not found")
}
