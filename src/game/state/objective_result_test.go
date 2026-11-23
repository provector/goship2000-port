package state

import (
	"encoding/binary"
	"testing"
)

func TestAssignedPickupUsesNormalizedActualActor(t *testing.T) {
	for _, tc := range []struct {
		actor    int
		cargo    byte
		control  byte
		leader   int
		near     uint32
		returned bool
	}{
		{1, 0x18, 1, 0, 2000, true}, {4, 0x18, 0, 0, 768, true}, {4, 0x18, 0, 0, 769, false},
		{1, 0, 1, 0, 2000, true}, {1, 0x10, 1, 0, 2000, true},
	} {
		f := contextFacts()
		f.ActorCount = 5
		f.SourceFlags = 0x80
		c := f.ResultContext
		c.Actors = make([]SortieActorFacts, 5)
		c.MissionRecovery = &MissionRecoveryContext{ObjectiveAssignments: [2]uint16{uint16(tc.actor + 1), 0}}
		for actor := range c.Actors {
			c.Actors[actor].Kind = 0x20
			c.Actors[actor].Pose = InitializedPose{H0: 2000 * 16}
		}
		a := &c.Actors[tc.actor]
		a.Control = tc.control
		a.Leader = tc.leader
		a.Ship[0] = 6 | tc.cargo
		a.Pose = InitializedPose{H0: tc.near * 16}
		a.Flags = 1
		binary.LittleEndian.PutUint32(f.Targets[0][42:], 0)
		binary.LittleEndian.PutUint32(f.Targets[0][46:], 0)
		binary.LittleEndian.PutUint16(f.Targets[2][32:], 0)
		binary.LittleEndian.PutUint16(f.Targets[3][32:], 4)
		if err := FinalizeSortieFacts(&f); err != nil {
			t.Fatal(err)
		}
		if (a.Flags&0x80 != 0) != tc.returned {
			t.Fatal("source return boundary/inheritance", tc, a.Flags)
		}
		if err := FinalizeAssignedObjectives(&f); err != nil {
			t.Fatal(err)
		}
		want := tc.returned && tc.cargo == 0x18
		if (f.SourceFlags&1 != 0) != want || c.Actors[0].Flags != f.SourceFlags || f.SourceFlags&2 != 0 {
			t.Fatal("assigned source SHIP/return consumer", tc, f.SourceFlags)
		}
	}
}

func TestAssignedPickupRejectsMissingActorAndKeepsOtherRole(t *testing.T) {
	f := contextFacts()
	f.ResultContext.MissionRecovery = &MissionRecoveryContext{ObjectiveAssignments: [2]uint16{2, 0}}
	if err := FinalizeAssignedObjectives(&f); err == nil {
		t.Fatal("missing assigned actor guessed")
	}
	f.ResultContext.MissionRecovery.ObjectiveAssignments = [2]uint16{}
	if err := FinalizeAssignedObjectives(&f); err != nil || f.SourceFlags&3 != 3 {
		t.Fatal("unassigned complete roles", f.SourceFlags, err)
	}
}
