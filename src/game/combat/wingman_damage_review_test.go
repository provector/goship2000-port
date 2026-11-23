package combat

import (
	"encoding/binary"
	"testing"
)

func TestNPCSingleEngineCriticalUsesOwnSourceSuccessorForEverySelector(t *testing.T) {
	for selector := byte(0); selector < 8; selector++ {
		for _, fall := range []bool{false, true} {
			s, in, sys := successorFixture(t)
			m := s.init
			e := &m.Entities[1]
			m.Stores.Ships[1].Flags = selector
			e.SerializedAircraftSelector = int(selector)
			*m.Stores.Ships[4].Proficiency = 50 // bucket4 owns this literal SHIP alias.
			m.Stores.Damage[1][2] = 1           //602 first injury supplied; actual next hit fails it.
			npcDamageRound(s, 20, 3, 0)
			words := []uint16{1, 77}
			if selector < 2 {
				words = append(words, 77)
			}
			roll := uint16(51)
			want609 := byte(2)
			if fall {
				roll = 50
				want609 = 1
			}
			words = append(words, roll)
			at := sequence(t, &in, words...)
			before := e.Pose
			if err := s.Step(in); err != nil {
				t.Fatal(selector, fall, err)
			}
			if *at != len(words) || m.Stores.Damage[1][2] != 2 || m.Stores.Damage[1][7] != 0 || m.Stores.Damage[1][9] != want609 || e.Pose != before || e.Kind != 0x20 || e.Participation != 1 {
				t.Fatal(selector, fall, *at, e, m.Stores.Damage12(1))
			}
			if _, err := s.AdvanceFollowers(in, sys); err != nil {
				t.Fatal(err)
			}
			if fall {
				if e.Participation != 2 || e.RawMotion != 4 {
					t.Fatal("6091 must enter source fall", selector, e)
				}
			} else if e.Kind != 0x26 || e.ControlRaw[1] != 6 || e.Participation != 1 {
				t.Fatal("6092 must enter source descent", selector, e)
			}
		}
	}
}

func TestNPCOneProjectileChangesTwoSystemsWithoutSecondActorUpdate(t *testing.T) {
	s, in := victimFixture(t)
	m := s.init
	m.Stores.Ships[1].Chaff = 0
	m.Stores.Damage[1][7] = 1 //already partial607.
	npcDamageRound(s, 20, 0x43, 0)
	// Actual contact; first602 partial; radar199 permits second607 critical;
	// its alias compact2[21] was frozen from the actual launch definition.
	alias := m.Stores.NPCDamage.BucketBytes[9]
	roll := uint16(alias) + 1
	if roll > 127 {
		roll = 127
	}
	at := sequence(t, &in, 1, 77, 199, 172, roll)
	before := m.Entities[1].Pose
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	want := byte(1)
	if byte(roll) > alias {
		want = 2
	}
	if *at != 5 || m.Stores.Damage[1][2] != 1 || m.Stores.Damage[1][7] != 2 || m.Stores.Damage[1][9] != want || m.Entities[1].Pose != before || m.Entities[1].Kind != 0x20 || s.Results[3].Target != 0xa5 || len(events(s, "notification")) != 2 {
		t.Fatal(*at, m.Stores.Damage12(1), s.Results, s.Events)
	}
	if len(events(s, "shake")) != 0 || len(events(s, "sound")) != 0 {
		t.Fatal("NPC hit borrowed player feedback", s.Events)
	}
}

func TestNPCCannonDamageConsumerKeepsPlayerAndNPCDistinct(t *testing.T) {
	for _, damage := range []byte{0, 1, 2, 0x82} {
		s, in := fireFixture(t)
		m := s.init
		ship := &m.Stores.Ships[1]
		ship.Stations[0] = ship.Stations[1]
		ship.Stations[0].Ammunition = 101 // ADD3..ADEE source burst consumes20.
		ship.Stations[1].Ammunition = 0
		m.Stores.Damage[1][4] = damage
		draws := 0
		in.Random = func() uint16 { draws++; return 127 }
		if err := s.advanceHealthyFollower(1, in); err != nil {
			t.Fatal(err)
		}
		if damage == 2 {
			if s.Slots[16].Lifetime != 0 || draws != 0 || ship.Stations[0].Ammunition != 101 {
				t.Fatal(damage, s.Slots[16], draws, ship)
			}
		} else if s.Slots[16].Lifetime == 0 || draws != 1 || ship.Stations[0].Ammunition != 81 {
			t.Fatal(damage, s.Slots[16], draws, ship)
		}
	}
}

func TestNPCPartialFuelBurnAndServiceKeepDamage(t *testing.T) {
	s, in, sys := successorFixture(t)
	m := s.init
	m.Stores.Damage[1][3] = 1
	m.Stores.NPCDamage.Masks[1] = [2]byte{1, 2}
	sys.Fuel[1] = 3
	sys.Countdown = 0
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if sys.Fuel[1] != 1 || m.Stores.Damage[1][9] != 0 {
		t.Fatal(sys.Fuel, m.Stores.Damage12(1))
	}
	sys.Countdown = 0
	sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(in.Random))
	if sys.Fuel[1] != 0 || m.Stores.Damage[1][9] != 0 {
		t.Fatal("oldpositive1->0 still powered", sys.Fuel, m.Stores.Damage12(1))
	}
	// Actual near-base6092 descent can service fuel but preserves all damage.
	e := &m.Entities[1]
	e.Pose.Height = 8
	e.Kind = 0x26
	e.Participation = 1
	e.ControlRaw[1] = 6
	m.Stores.Damage[1][9] = 2
	m.FollowerFailureKnown[1] = true
	binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], e.Pose.H0)
	binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], e.Pose.H1)
	for n := 0; n < 30 && e.Participation != 0; n++ {
		if _, err := s.AdvanceFollowers(in, sys); err != nil {
			t.Fatal(err)
		}
	}
	if e.Participation != 0 || m.Stores.Damage[1][9] != 0 || m.Stores.Damage[1][3] != 1 || m.Stores.NPCDamage.Masks[1] != [2]byte{1, 2} || sys.Fuel[1] != 100 {
		t.Fatal(e, m.Stores.Damage12(1), sys.Fuel)
	}
}
