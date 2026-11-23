package state

import "fmt"

// The row reference identifies a real launch entity even if it is a nondrawing
// formation/route anchor. Paper wording remains derived from MissionData; no
// renderer strings or separately generated objective positions are stored here.
type InitializedObjective struct {
	Role      string
	SourceRow int
	EntityID  string
	RawKind   int
}

func (m *InitializedMission) LinkObjectives(prepared *MissionData) error {
	if m == nil || prepared == nil {
		return fmt.Errorf("objective linkage needs initialized and prepared mission state")
	}
	links := make(map[string]InitializedObjective, len(prepared.Objectives))
	for role, o := range prepared.Objectives {
		if o.ObjectIndex < 0 || o.ObjectIndex >= len(m.Entities) {
			return fmt.Errorf("%s objective row%d outside initialized population", role, o.ObjectIndex)
		}
		entity := m.Entities[o.ObjectIndex]
		if entity.SourceRow != o.ObjectIndex {
			return fmt.Errorf("%s objective row%d lost source identity", role, o.ObjectIndex)
		}
		links[role] = InitializedObjective{Role: role, SourceRow: o.ObjectIndex, EntityID: entity.ID, RawKind: o.RawKind}
	}
	m.Objectives = links
	return nil
}
