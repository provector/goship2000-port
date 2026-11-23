package state

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
)

// ObjectiveDistance is SAR32/4, low16 subtraction/ABS16, wrapped word addition.
// It is the source query metric, not metres, AGL, or a mesh distance.
func ObjectiveDistance(record [62]byte, pose InitializedPose) uint16 {
	return SourceServiceDistance(MissionPoint{X: int32(binary.LittleEndian.Uint32(record[34:])), Y: int32(binary.LittleEndian.Uint32(record[38:]))}, flight.Pose{H0: pose.H0, H1: pose.H1, Height: int16(pose.Height), Angles: pose.Angles})
}

// ObjectiveRecovered follows source section0:116e..1230. Clearing a pickup goal
// is only a progress event: the same assigned actor must still carry18 and return80.
// Returned is consumer-owned because source follower result producers are separate.
func (m *InitializedMission) ObjectiveRecovered(role int, returned func(int) (byte, bool)) bool {
	if m == nil || m.Targets == nil || role < 0 || role > 1 || binary.LittleEndian.Uint16(m.Targets.Records[role+2][32:]) != 0 {
		return false
	}
	owner := m.ObjectiveAssignments[role]
	if owner == 0 {
		return true
	}
	row := int(owner) - 1
	if m.Stores == nil || row < 0 || row >= len(m.Stores.Ships) || m.Stores.Ships[row].Flags&0xf8 != 0x18 || returned == nil {
		return false
	}
	flags, known := returned(row)
	return known && flags&0x80 != 0
}

// FinalizePlayerObjectives publishes only source-recovered player facts at the
// frozen ending boundary. It does not score or manufacture follower return data.
func (m *InitializedMission) FinalizePlayerObjectives() {
	if m == nil || m.Targets == nil || m.Sortie == nil || m.Sortie.Result == nil {
		return
	}
	end := m.Sortie.Result
	flags := end.SourceStatus
	if end.Reason == "Alt+E" {
		flags &^= 0x80
	} // Native voluntary-end policy is not safe return.
	flags &^= 3
	returned := func(actor int) (byte, bool) {
		if actor == m.PlayerRow {
			return flags, true
		}
		return 0, false
	}
	if m.ObjectiveRecovered(0, returned) {
		flags |= 1
	}
	if m.ObjectiveRecovered(1, returned) {
		flags |= 2
	}
	end.SourceStatus = flags
}

// FinalizeAssignedObjectives is source0:116e..1230 after the ordered follower
// return normalizer. Existing historical callers retain FinalizePlayerObjectives.
// It consumes frozen actual SHIP/actor flags, not an inferred service/arrival bit.
func FinalizeAssignedObjectives(f *SortieFacts) error {
	if f == nil || f.ResultContext == nil || len(f.Targets) != 5 {
		return nil
	}
	c := f.ResultContext
	if len(c.Actors) != f.ActorCount || c.MissionRecovery == nil {
		return fmt.Errorf("assigned objective finalizer needs frozen actual actor/assignment owners")
	}
	f.SourceFlags &^= 3
	for role, owner := range c.MissionRecovery.ObjectiveAssignments {
		if binary.LittleEndian.Uint16(f.Targets[role+2][32:]) != 0 {
			continue
		}
		recovered := owner == 0
		if owner != 0 {
			actor := int(owner) - 1
			if actor < 0 || actor >= len(c.Actors) {
				return fmt.Errorf("objective%d assigned actor%d outside frozen flight", role, actor)
			}
			a := c.Actors[actor]
			recovered = a.Ship[0]&0xf8 == 0x18 && a.Flags&0x80 != 0
		}
		if recovered {
			f.SourceFlags |= 1 << uint(role)
		}
	}
	c.Actors[0].Flags = f.SourceFlags
	return nil
}
