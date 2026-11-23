package combat

import (
	"encoding/binary"
	"testing"
)

func TestCriticalActualContactFallAndRecovery(t *testing.T) {
	for _, recoverable := range []bool{false, true} {
		s, in, sys := successorFixture(t)
		m := s.init
		m.Stores.VictimCriticalProficiency = true
		*m.Stores.Ships[1].Proficiency = 50
		if recoverable {
			*m.Stores.Ships[1].Proficiency = 25
		}
		e := &m.Entities[1]
		e.RuntimeRow, e.FlightSlot = 1, 1 // Complete the supplied fixture's assigned-flight identity.
		// Supplied prior injury; real accepted contact consumes the same roll50.
		// Only the actual victim skill changes the fall versus descent successor.
		m.Stores.Damage[1][0] = 1
		m.Stores.NPCDamage.Masks[1][0] = 1
		binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], e.Pose.H0)
		binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], e.Pose.H1)
		npcDamageRound(s, 20, 3, 0)
		roll := uint16(50)
		at := sequence(t, &in, 1, 44, roll)
		before := e.Pose
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		want609 := byte(1)
		if recoverable {
			want609 = 2
		}
		if *at != 3 || e.Pose != before || e.Kind != 0x20 || e.Participation != 1 || m.Stores.Damage[1][9] != want609 {
			t.Fatal("damage must not redispatch aircraft in projectile phase", recoverable, e, m.Stores.Damage[1], *at)
		}
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
		if recoverable {
			if e.Kind != 0x26 || e.Participation != 1 || e.Pose.Height != 350 {
				t.Fatal("next callback must select descent and retain old20 geometry", e)
			}
			for tick := 0; tick < 100 && e.Participation != 0; tick++ {
				if _, err := s.AdvanceFollowers(in, sys); err != nil {
					t.Fatal(err)
				}
			}
			if e.Kind != 0x20 || e.Participation != 0 || e.Pose.Height != 0 || e.ControlRaw[1] != 2 || m.Entities[5].Participation != 0 || m.Stores.Damage[1][9] != 0 || sys.Fuel[1] != 100 {
				t.Fatal("actual near-base descent/service must recover without inventing SOS", e, m.Stores.Damage[1], sys.Fuel)
			}
			if m.Stores.Damage12(1)[0] != 1 || m.Stores.Damage[1][0] != 2 {
				t.Fatal("service must retain masks and subsystem damage", m.Stores.Damage12(1))
			}
		} else {
			if e.Kind != 0x20 || e.Participation != 2 || e.Pose.Height != 350 {
				t.Fatal("next callback must select fall without same-visit fall update", e)
			}
			for tick := 0; tick < 100 && e.Kind != 0x1c; tick++ {
				if _, err := s.AdvanceFollowers(in, sys); err != nil {
					t.Fatal(err)
				}
			}
			if e.Kind != 0x1c || e.Participation != 0 || e.ControlRaw[1] != 0 || e.ModelID != e.ReplacementModelID || s.Results[3].Kind != 3 || s.Results[3].Actor != 1 {
				t.Fatal("actual fall/contact must publish wreck and event3", e, s.Results)
			}
		}
	}
}
