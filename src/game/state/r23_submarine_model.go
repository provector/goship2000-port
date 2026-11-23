package state

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/world"
)

// InitializeSubmarineModels admits the new source model/controller owner before
// launch checkpointing. The old eager appearance initializer remains unchanged
// for historical films. Source C118 must still get its real first actor visit.
func (m *InitializedMission) InitializeSubmarineModels(w *world.Bundle) error {
	for row := range m.Entities {
		e := &m.Entities[row]
		if e.Kind != 0x0a {
			continue
		}
		private, err := world.NewSubmarineModel(w, e.ModelID)
		if err != nil {
			return fmt.Errorf("submarine row%d: %w", row, err)
		}
		if m.SubmarineModels == nil {
			m.SubmarineModels = map[int]*world.MutableModel{}
		}
		m.SubmarineModels[row] = private
		e.Participation, e.RawMotion = 0, 0
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
		m.syncEntity(row)
	}
	return nil
}

func (m *InitializedMission) EnsureSubmarineModel(row int, w *world.Bundle) (*world.MutableModel, error) {
	if row < 0 || row >= len(m.Entities) || m.Entities[row].Kind != 0x0a {
		return nil, fmt.Errorf("private submarine row%d is not kind0A", row)
	}
	if private := m.SubmarineModels[row]; private != nil {
		if private.ModelID != m.Entities[row].ModelID {
			return nil, fmt.Errorf("private submarine row%d model changed without replacement", row)
		}
		return private, nil
	}
	private, err := world.NewSubmarineModel(w, m.Entities[row].ModelID)
	if err != nil {
		return nil, err
	}
	if m.SubmarineModels == nil {
		m.SubmarineModels = map[int]*world.MutableModel{}
	}
	m.SubmarineModels[row] = private
	return private, nil
}

func (m *InitializedMission) ClearSubmarineModel(row int) {
	delete(m.SubmarineModels, row)
	if len(m.SubmarineModels) == 0 {
		m.SubmarineModels = nil
	}
}
