package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"testing"
)

// Supplied contacts exercise the real NPC7468 producer and existing dispatcher
// successors. This is separate from the normally launched natural-shot witness.
func successorFixture(t *testing.T) (*State, Input, *flight.Systems) {
	t.Helper()
	s, in, sys := followerFixture(t)
	s.SetLogisticsEnabled(true)
	s.SetWingmanDamageEnabled(true)
	s.init.Switches = 3
	for actor := 0; actor < 5; actor++ {
		s.init.Entities[actor].SerializedAircraftSelector = 6
		if actor > 1 {
			s.init.Entities[actor].ControlRaw[1] = 0
		}
	}
	if err := s.init.Stores.EnableNPCDamage(make([]byte, 80)); err != nil {
		t.Fatal(err)
	}
	in.Incoming = &IncomingContext{Defenses: &flight.Defenses{}, Systems: sys,
		ApplyPlayerHit: func(int) (uint16, error) { t.Fatal("NPC successor reached player effects"); return 0, nil }}
	return s, in, sys
}

func TestCriticalDamageEntersNextActualFollowerVisit(t *testing.T) {
	for _, recoverable := range []bool{false, true} {
		s, in, sys := successorFixture(t)
		m := s.init
		e := &m.Entities[1]
		// One prior602 severity is supplied; the new actual gun contact takes
		// it to2. SHIP[bucket2]+1D is0, so draw0/1 selects6091/2.
		m.Stores.Damage[1][0] = 1
		m.Stores.NPCDamage.Masks[1][0] = 1
		binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], e.Pose.H0)
		binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], e.Pose.H1)
		npcDamageRound(s, 20, 3, 0)
		roll := uint16(0)
		if recoverable {
			roll = 1
		}
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

func TestRadarSecondDamageRetainsSameWrapperAfterFuelWreck(t *testing.T) {
	s, in := victimFixture(t)
	m := s.init
	m.Stores.Damage[1][3] = 1
	m.Stores.Ships[1].Chaff, m.Stores.Ships[1].Fuel = 0, 99
	npcDamageRound(s, 20, 0x43, 0)
	// First7468 selects603 fuel and directly wrecks.95BE still calls the
	// second7468; it selects mask0, rather than reclassifying to9D1C.
	at := sequence(t, &in, 1, 95, 199, 0)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	e := &m.Entities[1]
	if *at != 4 || e.Kind != 0x1c || e.Participation != 0 || m.Stores.Damage12(1)[0] != 1 || m.Stores.Damage[1][3] != 2 || len(events(s, "notification")) != 2 || s.Results[3].Target != 0x16 {
		t.Fatal("same-wrapper second call lost after replacement", *at, e, m.Stores.Damage12(1), s.Events, s.Results)
	}
	if _, err := s.AdvanceFollowers(in, in.Incoming.Systems); err != nil {
		t.Fatal(err)
	}
	if e.Participation != 4 || e.Pose.Height != 0 {
		t.Fatal("next actual wreck visit must settle model at terrain", e)
	}
}
