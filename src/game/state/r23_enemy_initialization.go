package state

import (
	"encoding/binary"
	"gunship2000-assets/game/world"
)

// InitializeEnemyBehaviors is called only for a new R23 interpretation, before
// initial checkpointing. Import's position/cardinal-heading draw is retained.
// B6F2's first actual actor visit, rather than an eager launch fold, owns P1.
func (m *InitializedMission) InitializeEnemyBehaviors(w *world.Bundle) error {
	for row := range m.Entities {
		e := &m.Entities[row]
		if e.Kind != 0 && e.Kind != 1 {
			continue
		}
		e.Participation, e.RawMotion, e.Pose.Height = 0, 0, 0
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
		m.syncEntity(row)
	}
	return m.InitializeSubmarineModels(w)
}
