package combat

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

// Supplied geometry/RNG fixtures are deliberately separate from the real A070
// constructor case below and the normally launched encounter integration test.
func victimFixture(t *testing.T) (*State, Input) {
	t.Helper()
	s, in, _ := attackFixture(t)
	stores := s.init.Stores
	stores.Ships = make([]gamestate.RuntimeAircraft, 5)
	for i := range stores.Ships {
		proficiency := byte(127)
		stores.Ships[i] = gamestate.RuntimeAircraft{Flags: 2, Fuel: 100, Chaff: 30, Flares: 30, Proficiency: &proficiency}
	}
	stores.FreshShipPadding = true
	stores.Damage = make([][10]byte, 5)
	if err := stores.EnableNPCDamage(make([]byte, 80)); err != nil {
		t.Fatal(err)
	}
	s.SetWingmanDamageEnabled(true)
	s.init.Switches = 3
	target, shooter := &s.init.Entities[1], &s.init.Entities[2]
	target.Kind, target.Participation, target.ControlRaw[1], target.SerializedAircraftSelector, target.FlightSlot = 0x20, 1, 2, 2, 1
	target.ModelID, target.ReplacementModelID = 0, 1
	s.assets.Models = []string{"live", "wreck"}
	shooter.Kind, shooter.Participation, shooter.DynamicRaw[2], shooter.Object.Raw[11] = 2, 1, 0, 0x22
	shooter.Pose = target.Pose
	shooter.Pose.H1 -= 1024
	in.Contacts[2].Flags = 2 | 1<<3
	systems := flight.NewSystems([]int16{100, 100, 100, 100, 100}, []bool{true, true, true, true, true})
	in.Incoming.Systems = &systems
	in.Incoming.ApplyPlayerHit = func(int) (uint16, error) { t.Fatal("NPC reached player effects"); return 0, nil }
	return s, in
}

func npcDamageRound(s *State, slot int, flags uint16, threshold byte) {
	p := Projectile{Flags: flags, Lifetime: 10, Shooter: 2, Target: 1, Speed: 64, Packed: threshold, Pose: s.init.Entities[1].Pose}
	p.Pose.H1 -= 64
	p.Pose.Angles = [3]uint16{} // Source yaw0 advances toward this supplied target.
	s.Slots[slot], s.incomingAdmitted[slot] = p, true
}

func sequence(t *testing.T, in *Input, words ...uint16) *int {
	t.Helper()
	at := 0
	in.Random = func() uint16 {
		if at >= len(words) {
			t.Fatalf("unexpected draw%d, expected%v", at, words)
		}
		v := words[at]
		at++
		return v
	}
	return &at
}

func TestExactCampaignContactDefenseOperands(t *testing.T) {
	s, in := victimFixture(t)
	npcDamageRound(s, 22, 0x83, 0)
	state := uint16(0xaeeb)
	draws := 0
	in.Random = func() uint16 {
		if state&1 != 0 {
			state = state>>1 ^ 0xb400
		} else {
			state >>= 1
		}
		draws++
		return state
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if state != 0xc5ba || draws != 2 || s.init.Stores.Ships[1].Flares != 27 || s.Slots[22].Packed != 0 || s.Slots[22].Lifetime != 0 {
		t.Fatal(state, draws, s.init.Stores.Ships[1], s.Slots[22])
	}
	if s.init.Stores.Damage12(1) != [12]byte{} || len(events(s, "notification")) != 0 || s.Results[3].Sequence != 0 || len(events(s, "shake")) != 0 || len(events(s, "sound")) != 0 {
		t.Fatal("defense is not damage", s.Events, s.Results, s.init.Stores.Damage12(1))
	}
}

func TestNaturalGunDamageAndThresholdEquality(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			s, in := victimFixture(t)
			words := []uint16{1, 230}
			if accepted {
				words = []uint16{1, 255, 0}
			}
			at := sequence(t, &in, words...)
			beforePose, beforeControl := s.init.Entities[1].Pose, s.init.Entities[1].ControlRaw
			if got, err := s.stationaryEnemyAttack(2, in); err != nil || !got {
				t.Fatal(got, err)
			}
			for tick := 0; tick < 20 && s.Slots[21].Lifetime != 0; tick++ {
				if err := s.Step(in); err != nil {
					t.Fatal(tick, err)
				}
			}
			if *at != len(words) || s.Slots[21].Lifetime != 0 {
				t.Fatal(*at, s.Slots[21])
			}
			d := s.init.Stores.Damage12(1)
			health := gamestate.EncodedDamageHealth(d)
			if accepted {
				if d[0] != 1 || health != 242 || len(events(s, "notification")) != 1 || s.Results[3].Kind != 2 || s.Results[3].Actor != 1 || s.Results[3].Target != 1 {
					t.Fatal(d, health, s.Events, s.Results)
				}
			} else if d != [12]byte{} || health != 255 || len(events(s, "notification")) != 0 || s.Results[3].Sequence != 0 {
				t.Fatal(d, health, s.Events, s.Results)
			}
			if s.init.Stores.Damage[1] != [10]byte{} || s.init.Entities[1].Pose != beforePose || s.init.Entities[1].ControlRaw != beforeControl || len(events(s, "shake")) != 0 {
				t.Fatal("nonfatal mask-only damage changed flight/player", s.init.Entities[1])
			}
		})
	}
}

func TestDefenseSignedStoresEqualityOrderAndPolicies(t *testing.T) {
	for _, flags := range []uint16{0x83, 0x43, 0x8083} {
		for _, store := range []int16{-1, 0, 1} {
			for _, value := range []uint16{127, 128} {
				t.Run(fmt.Sprintf("%x/%d/%d", flags, store, value), func(t *testing.T) {
					s, in := victimFixture(t)
					npcDamageRound(s, 20, flags, 0)
					ship := &s.init.Stores.Ships[1]
					ship.Chaff, ship.Flares = store, store
					words := []uint16{1}
					defended := store > 0 && value > 127
					if store > 0 {
						words = append(words, value)
					}
					if !defended {
						words = append(words, 0)
						if flags&0x40 != 0 {
							words = append(words, 200)
						}
					}
					at := sequence(t, &in, words...)
					s.enemyAutonomyDisabled = true
					s.SetWingmenAutonomyEnabled(false)
					s.init.Switches = 3
					if err := s.Step(in); err != nil {
						t.Fatal(err)
					}
					want := store
					if store > 0 {
						want -= 3
					}
					actual := ship.Flares
					if flags&0x40 != 0 {
						actual = ship.Chaff
					}
					if actual != want || *at != len(words) || (s.init.Stores.Damage12(1) == [12]byte{}) != defended {
						t.Fatal(actual, want, *at, words, s.init.Stores.Damage12(1))
					}
				})
			}
		}
	}
	// Radar success exits before the IR arm, even with bypass8000 and both bits.
	s, in := victimFixture(t)
	npcDamageRound(s, 20, 0x80c3, 0)
	at := sequence(t, &in, 1, 128)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if *at != 2 || s.init.Stores.Ships[1].Chaff != 27 || s.init.Stores.Ships[1].Flares != 30 {
		t.Fatal(*at, s.init.Stores.Ships[1])
	}
	// Damaged defenses skip both store/draw arms; this mask must survive damage.
	s, in = victimFixture(t)
	npcDamageRound(s, 20, 0xc3, 0)
	s.init.Stores.NPCDamage.Masks[1][0] = 0x40
	at = sequence(t, &in, 1, 0, 200)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if *at != 3 || s.init.Stores.Ships[1].Chaff != 30 || s.init.Stores.Ships[1].Flares != 30 || s.init.Stores.Damage12(1)[0] != 0x41 {
		t.Fatal(*at, s.init.Stores.Ships[1], s.init.Stores.Damage12(1))
	}
}

func TestLaterSlotsSeeSpentStoresAndActualDamage(t *testing.T) {
	s, in := victimFixture(t)
	s.init.Stores.Ships[1].Flares = 1
	npcDamageRound(s, 20, 0x83, 0)
	npcDamageRound(s, 21, 0x83, 0)
	at := sequence(t, &in, 1, 128, 1, 0)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if *at != 4 || s.init.Stores.Ships[1].Flares != -2 || s.Slots[20].Packed != 0 || s.Slots[21].Packed != 2 || s.init.Stores.Damage12(1)[0] != 1 || len(events(s, "notification")) != 1 {
		t.Fatal(*at, s.Slots, s.init.Stores, s.Events)
	}
}

func TestLiveDefenseIndependentOfCPGAndBothAIPolicies(t *testing.T) {
	for _, cpg := range []uint16{0, 8, 0x18} {
		for _, enemyDisabled := range []bool{false, true} {
			for _, wingmenDisabled := range []bool{false, true} {
				s, in := victimFixture(t)
				s.init.Switches = 3 | cpg
				s.enemyAutonomyDisabled = enemyDisabled
				s.SetWingmenAutonomyEnabled(!wingmenDisabled)
				npcDamageRound(s, 20, 0x83, 0)
				at := sequence(t, &in, 1, 128)
				if err := s.Step(in); err != nil {
					t.Fatal(cpg, enemyDisabled, wingmenDisabled, err)
				}
				if *at != 2 || s.init.Stores.Ships[1].Flares != 27 || s.init.Stores.Damage12(1) != [12]byte{} || s.Slots[20].Packed != 0 {
					t.Fatal(cpg, enemyDisabled, wingmenDisabled, *at, s.init.Stores.Damage12(1))
				}
			}
		}
	}
}

func TestRadarSecondOpportunityAndPackedEvent(t *testing.T) {
	for _, second := range []bool{false, true} {
		s, in := victimFixture(t)
		npcDamageRound(s, 20, 0x43, 0)
		s.init.Stores.Ships[1].Chaff = 0
		words := []uint16{1, 0, 200}
		if second {
			words = []uint16{1, 0, 199, 26}
		}
		at := sequence(t, &in, words...)
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		want := byte(1)
		notices := 1
		if second {
			want = 0x21
			notices = 2
		}
		if *at != len(words) || s.Results[3].Target != want || len(events(s, "notification")) != notices {
			t.Fatal(*at, s.Results, s.Events)
		}
	}
	// Poor quality suppresses the first actual7468 call, but95BE still draws
	//199 and admits the second call. Its resulting code belongs in the high nibble.
	s, in := victimFixture(t)
	s.init.Switches = 0
	npcDamageRound(s, 20, 0x43, 0)
	s.init.Stores.Ships[1].Chaff = 0
	at := sequence(t, &in, 1, 0, 199, 76, 0)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if *at != 5 || s.Results[3].Target != 0x10 || len(events(s, "notification")) != 1 {
		t.Fatal(*at, s.Results, s.Events)
	}
}

func TestDirectWreckThenLaterLiveSlotUsesInactiveFixedLeaf(t *testing.T) {
	s, in := victimFixture(t)
	s.init.Stores.Damage[1][3] = 1
	s.init.Stores.Ships[1].Fuel = 99
	s.init.Entities[1].DynamicRaw[2] = 0x31
	npcDamageRound(s, 20, 3, 0)
	npcDamageRound(s, 21, 0x43, 0)
	at := sequence(t, &in, 1, 95, 1)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	e := s.init.Entities[1]
	if *at != 3 || e.Kind != 0x1c || e.Participation != 0 || e.ModelID != 1 || e.DynamicRaw[2] != 0 || e.ControlRaw[1] != 0 || binary.LittleEndian.Uint16(e.DynamicRaw[4:]) != 0xff9c || s.Slots[21].Packed != 2 {
		t.Fatal(*at, e, s.Slots[21])
	}
	if s.init.Stores.Ships[1].Fuel != 98 || in.Incoming.Systems.Fuel[1] != 98 || len(events(s, "notification")) != 1 || s.Results[3].Target != 6 || s.Results[3].Sequence != 1 {
		t.Fatal(s.init.Stores.Ships[1], s.Events, s.Results)
	}
	//ActiveP1 remains named/guarded; all exact negative phases leave state alone.
	for _, phase := range []byte{0, 1, 2, 3, 4} {
		s, in := victimFixture(t)
		npcDamageRound(s, 20, 3, 0)
		s.init.Entities[1].Kind, s.init.Entities[1].Participation = 0x1c, phase
		before := s.init.Entities[1]
		at := sequence(t, &in, 1)
		err := s.Step(in)
		if (err != nil) != (phase == 1) || *at != 1 || !reflect.DeepEqual(before, s.init.Entities[1]) {
			t.Fatal(phase, err, *at, s.init.Entities[1])
		}
		if phase != 1 && (s.Slots[20].Packed != 2 || s.Results[3].Sequence != 0 || len(events(s, "notification")) != 0) {
			t.Fatal(phase, s.Slots[20], s.Events)
		}
	}
}
