package state

import (
	"encoding/binary"
	"gunship2000-assets/game/world"
)

// ContactObjects retains the primary loader's query identity independently of
// the current visual header. Replacement actorFFFF does not register height
// regions; the later wreck initializerDD2E clears that row's original prefix.
func (m *InitializedMission) ContactObjects() []world.QueryObject {
	var out []world.QueryObject
	for _, e := range m.Entities {
		if (e.Kind == 0x1c || e.Kind == 0x1d) && e.Participation == 4 {
			continue
		}
		flags := binary.LittleEndian.Uint16(e.Object.Raw[:])
		if flags&2 == 0 || flags&4 != 0 {
			out = append(out, world.QueryObject{ModelID: int(e.Object.Raw[12]), Pose: world.QueryPose{H0: e.Pose.H0, H1: e.Pose.H1, Height: int16(e.Pose.Height)}})
		}
	}
	return out
}
