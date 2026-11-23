package combat

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"

	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

func fixedFixture(t *testing.T, victim int) (*State, Input) {
	t.Helper()
	s, in := victimFixture(t)
	s.enemyBehaviorEnabled = true
	s.assets.Geometries["live"] = s.assets.Geometries["flat"]
	s.assets.Geometries["wreck"] = s.assets.Geometries["flat"]
	e := &s.init.Entities[victim]
	e.Kind, e.Participation, e.RawMotion = 0x12, 1, 0
	e.DynamicRaw = [24]byte{}
	e.Object.Raw[0], e.Object.Raw[10] = 0, 0x12
	binary.LittleEndian.PutUint16(e.Object.Raw[27:], 37)
	binary.LittleEndian.PutUint16(e.Object.Raw[29:], 8)
	s.syncDeath(e)
	in.Phase = 73
	return s, in
}

func fixedRound(s *State, slot, victim, shooter int, flags uint16, threshold byte) {
	p := Projectile{Flags: flags, Lifetime: 10, Shooter: byte(shooter), Target: byte(victim), Speed: 64, Packed: threshold, Pose: s.init.Entities[victim].Pose}
	p.Pose.H1 -= 64
	p.Pose.Angles = [3]uint16{}
	s.Slots[slot], s.incomingAdmitted[slot] = p, true
}

func TestFixedDamageSourceOperands(t *testing.T) {
	// sourceTables binds the entire executable SHA before these byte checks.
	_ = sourceTables(t)
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, encoded := range map[int]string{
		0x94d4: "26f687822510",             // actual victim kind bit, after player precedence.
		0x94ee: "6b5eee1a26c687050002",     // fixed handler returns before tail2.
		0x9d2f: "2680bf832501",             // onlyP1 may access damage/score/loss state.
		0x9d46: "268b87690b",               // signed OBJECT+29 threshold.
		0x9d58: "268b8f622e",               // quality-indexed2E62 word.
		0x9d64: "26018f862526398786257d03", // wrapped ADD, signed JGE/equality.
		0x9db8: "26f6874c0b80",             // own OBJECT+0 flag80, not a model-name rule.
		0x9dc4: "26ff06f10a",               // once-only AF1 writer after P1->2.
		0x9e04: "9ac20b0000",               // event1(shooter,victim).
		0x9e3a: "24f60c06",                 // literal even-row packed credit6 mask.
	} {
		want, _ := hex.DecodeString(encoded)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("GS2 canonical0975:%04X operand changed", ip)
		}
	}
	if !bytes.Equal(b[0x487f0+0x2e62:][:8], []byte{2, 0, 4, 0, 6, 0, 8, 0}) {
		t.Fatal("quality fixed-contact table is not2/4/6/8")
	}
}

func TestFixedHitQualityThresholdsAndNoDefenseDraw(t *testing.T) {
	for quality := uint16(0); quality < 4; quality++ {
		increment := int16(2 * (quality + 1))
		for _, equal := range []bool{false, true} {
			s, in := fixedFixture(t, 1)
			s.init.Switches = 0xfffc | quality
			threshold := increment + 1
			if equal {
				threshold = increment
			}
			binary.LittleEndian.PutUint16(s.init.Entities[1].Object.Raw[29:], uint16(threshold))
			// Both radar/IR and8000 are deliberately set. Fixed victims do
			// not take NPC defenses or the NPC radar-second opportunity.
			fixedRound(s, 20, 1, 2, 0x80c3, 0)
			beforeChaff, beforeFlares := s.init.Stores.Ships[1].Chaff, s.init.Stores.Ships[1].Flares
			at := sequence(t, &in, 255)
			if err := s.Step(in); err != nil {
				t.Fatal(quality, equal, err)
			}
			e := s.init.Entities[1]
			if *at != 1 || s.Slots[20].Packed != 2 || s.Slots[20].Lifetime != 0 || s.init.Stores.Ships[1].Chaff != beforeChaff || s.init.Stores.Ships[1].Flares != beforeFlares || s.init.Stores.Damage12(1) != [12]byte{} || len(events(s, "shake")) != 0 {
				t.Fatal("fixed victim borrowed NPC/player state or draws", quality, equal, *at, s.Slots[20], s.Events)
			}
			if equal {
				if e.Participation != 2 || e.RawMotion != 4 || binary.LittleEndian.Uint16(e.DynamicRaw[4:]) != 0 || s.Score != 37 || s.Results[3] != (Result{Sequence: 1, Kind: 1, Actor: 2, Target: 1, Timestamp: 73}) || s.Credit[0] != 0x60 {
					t.Fatal("equality did not enter actual pending destruction", quality, e, s.Score, s.Results, s.Credit)
				}
				if got := events(s, "sound"); len(got) != 1 || got[0].Command != 15 {
					t.Fatal("fixed destruction cue", got)
				}
			} else if e.Participation != 1 || int16(binary.LittleEndian.Uint16(e.DynamicRaw[4:])) != increment || s.Score != 0 || s.Results[3].Sequence != 0 || len(events(s, "sound")) != 0 {
				t.Fatal("below threshold changed destruction facts", quality, e, s.Score, s.Results)
			}
		}
	}
}

func TestFixedWrappedAccumulatorAndScore(t *testing.T) {
	for _, tc := range []struct {
		acc, threshold int16
		quality        uint16
		wantAcc        int16
		dead           bool
	}{{32766, 1, 0, -32768, false}, {-4, 0, 1, 0, true}, {0, -1, 0, 0, true}} {
		s, in := fixedFixture(t, 1)
		e := &s.init.Entities[1]
		s.init.Switches = tc.quality
		s.Score = 32760
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], uint16(tc.acc))
		binary.LittleEndian.PutUint16(e.Object.Raw[29:], uint16(tc.threshold))
		fixedRound(s, 20, 1, 2, 3, 0)
		sequence(t, &in, 1)
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		if int16(binary.LittleEndian.Uint16(e.DynamicRaw[4:])) != tc.wantAcc || (e.Participation == 2) != tc.dead {
			t.Fatal(tc, e)
		}
		wantScore := int16(32760)
		if tc.dead {
			wantScore = -32739 // word32760+37 wraps.
		}
		if s.Score != wantScore {
			t.Fatal(tc, s.Score)
		}
	}
}

func TestFixedActualKindGateAndNeighborCredit(t *testing.T) {
	for _, victim := range []int{1, 2} {
		for _, kind := range []byte{0x10, 0x11, 0x12, 0x16, 0x1b, 0x1c} {
			s, in := fixedFixture(t, victim)
			s.init.Entities[victim].Kind = kind
			beforeShip := s.init.Stores.Ships[victim]
			s.Credit[0], s.Credit[1] = 0xab, 0xcd
			shooter := 3
			fixedRound(s, 20, victim, shooter, 3, 0)
			sequence(t, &in, 1)
			if err := s.Step(in); err != nil {
				t.Fatal(victim, kind, err)
			}
			if s.init.Entities[victim].Kind != kind || s.init.Entities[victim].Participation != 2 || !reflect.DeepEqual(beforeShip, s.init.Stores.Ships[victim]) {
				t.Fatal("kind10 branch is not a wingman SHIP writer or same-tick model replacement", kind, s.init.Entities[victim])
			}
			if victim == 1 && (s.Credit[0] != 0x6b || s.Credit[1] != 0xcd) || victim == 2 && (s.Credit[0] != 0xab || s.Credit[1] != 0xc6) {
				t.Fatal("source credit6 damaged neighboring nibble", victim, s.Credit)
			}
		}
	}
}

func TestFixedOuterEqualityHistoricalAndInactiveControls(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enabled    bool
		phase      byte
		rejected   bool
		wantError  bool
		wantPacked byte
	}{{"historical-active-guard", false, 1, false, true, 0}, {"historical-inactive-noop", false, 0, false, false, 2}, {"new-inactive-noop", true, 2, false, false, 2}, {"outer-equality-reject", true, 1, true, false, 0}} {
		s, in := fixedFixture(t, 1)
		s.enemyBehaviorEnabled = tc.enabled
		s.init.Entities[1].Participation = tc.phase
		before := s.init.Entities[1]
		threshold := byte(0)
		if tc.rejected {
			threshold = 117
		}
		fixedRound(s, 20, 1, 2, 0x43, threshold)
		at := sequence(t, &in, 117)
		err := s.Step(in)
		if (err != nil) != tc.wantError || *at != 1 || !reflect.DeepEqual(before, s.init.Entities[1]) || s.Score != 0 || s.Results[3].Sequence != 0 || len(events(s, "fixed-damage")) != 0 {
			t.Fatal(tc.name, err, *at, before, s.init.Entities[1], s.Events)
		}
		if !tc.wantError && s.Slots[20].Packed != tc.wantPacked {
			t.Fatal(tc.name, s.Slots[20])
		}
	}
}

func TestFixedFARPLossNoticesAndRepeatHit(t *testing.T) {
	for _, tc := range []struct {
		old, limit, want int16
		notice           uint16
	}{{0, 2, 1, 15}, {1, 2, 2, 14}, {2, 2, 3, 0}, {-1, 0, 0, 14}, {32767, -32768, -32768, 14}} {
		s, in := fixedFixture(t, 1)
		s.init.Entities[1].Object.Raw[0] = 0x80
		s.init.Sortie = &gamestate.SortieState{FARPLosses: tc.old}
		s.init.Targets = &gamestate.MissionTargets{}
		binary.LittleEndian.PutUint16(s.init.Targets.Records[1][50:], uint16(tc.limit))
		// Two actual slot visits:20 destroys,21 sees the newP2 and returns.
		fixedRound(s, 20, 1, 2, 0xc3, 0)
		fixedRound(s, 21, 1, 2, 0xc3, 0)
		at := sequence(t, &in, 255, 255)
		if err := s.Step(in); err != nil {
			t.Fatal(tc, err)
		}
		if *at != 2 || s.init.Sortie.FARPLosses != tc.want || s.Score != 37 || s.Results[3].Sequence != 1 || s.Slots[20].Packed != 2 || s.Slots[21].Packed != 2 || len(events(s, "fixed-damage")) != 1 || len(events(s, "destroyed")) != 1 {
			t.Fatal("later slot repeated consequence", tc, *at, s.init.Sortie, s.Score, s.Results, s.Events)
		}
		notices := events(s, "notification")
		if tc.notice == 0 && len(notices) != 0 || tc.notice != 0 && (len(notices) != 1 || notices[0].Notification != tc.notice || notices[0].Actor != 0 || notices[0].Target != 0) {
			t.Fatal(tc, notices)
		}
		// Repeat on a later tick too. Only outer acceptance may draw.
		before, _ := json.Marshal(s.init.Sortie)
		fixedRound(s, 22, 1, 2, 3, 0)
		sequence(t, &in, 255)
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s.init.Sortie)
		if !bytes.Equal(before, after) || s.Results[3].Sequence != 1 || s.Score != 37 {
			t.Fatal("repeat contact changed loss/score/event", tc)
		}
	}
}

func TestFixedFARPDestructionClosesExistingServiceGate(t *testing.T) {
	s, in, systems := followerFixture(t)
	s.enemyBehaviorEnabled = true
	s.SetWingmanDamageEnabled(true)
	s.init.Switches = 3
	m := s.init
	m.PlayerFlight = &flight.State{Pose: flight.Pose{H0: 0x40000, H1: 0x40000}, ValidContact: true}
	base := gamestate.MissionPoint{X: 0x60000, Y: 0x60000}
	mission := &gamestate.MissionData{Objectives: map[string]gamestate.MissionObjective{"base": {Point: base}}}
	binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], uint32(base.X))
	binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], uint32(base.Y))
	farp := &m.Targets.Records[1]
	binary.LittleEndian.PutUint16(farp[32:], 1)
	binary.LittleEndian.PutUint16(farp[50:], 1)
	binary.LittleEndian.PutUint32(farp[42:], m.PlayerFlight.Pose.H0)
	binary.LittleEndian.PutUint32(farp[46:], m.PlayerFlight.Pose.H1)
	if serviced, err := m.AdvanceService(mission, systems); err != nil || !serviced || m.Sortie.Services != 1 {
		t.Fatal("before damage FARP must use actual service owner", serviced, err, m.Sortie)
	}
	e := &m.Entities[9]
	e.Kind, e.Participation, e.Object.Raw[0] = 0x16, 1, 0x80
	binary.LittleEndian.PutUint16(e.Object.Raw[29:], 8)
	in.Random = func() uint16 { t.Fatal("9D1C itself consumes no RNG"); return 0 }
	if err := s.resolveFixedContact(9, 10, 20, in); err != nil {
		t.Fatal(err)
	}
	beforeShip, beforeServices := m.Stores.Ships[0], m.Sortie.Services
	if serviced, err := m.AdvanceService(mission, systems); err != nil || serviced || m.Sortie.FARPLosses != 1 || m.Sortie.FARPStatus != "limit reached" || m.Sortie.Services != beforeServices || !reflect.DeepEqual(beforeShip, m.Stores.Ships[0]) {
		t.Fatal("destruction did not close service at source equality", serviced, err, m.Sortie)
	}
}

func TestFixedPlayerPrecedenceAndLivePolicyIndependence(t *testing.T) {
	// Actual playerFF also contains bit10. It must reach player7468 first.
	s, in := fixedFixture(t, 1)
	playerCalls := 0
	in.Incoming.ApplyPlayerHit = func(int) (uint16, error) { playerCalls++; return 1, nil }
	fixedRound(s, 20, 0, 2, 3, 0)
	sequence(t, &in, 255)
	if err := s.Step(in); err != nil || playerCalls != 1 || len(events(s, "fixed-damage")) != 0 || len(events(s, "shake")) != 1 || s.Results[3].Kind != 2 || s.Results[3].Actor != 0 {
		t.Fatal("kindFF player lost precedence", err, playerCalls, s.Events, s.Results)
	}
	var canonical []byte
	for _, cpg := range []uint16{0, 8, 0x18} {
		for _, enemyOff := range []bool{false, true} {
			for _, wingmenOff := range []bool{false, true} {
				for _, recording := range []bool{false, true} {
					s, in := fixedFixture(t, 1)
					s.init.Switches = 3 | cpg
					s.enemyAutonomyDisabled = enemyOff
					s.SetWingmenAutonomyEnabled(!wingmenOff)
					seen := 0
					if recording {
						s.ResultObserver = func(Result) { seen++ }
					}
					fixedRound(s, 20, 1, 2, 3, 0)
					at := sequence(t, &in, 255)
					if err := s.Step(in); err != nil || *at != 1 || seen != map[bool]int{false: 0, true: 1}[recording] {
						t.Fatal(cpg, enemyOff, wingmenOff, recording, err, *at, seen)
					}
					encoded, _ := json.Marshal(s.FilmState())
					if canonical == nil {
						canonical = encoded
					} else if !bytes.Equal(canonical, encoded) {
						t.Fatal("live fixed hit changed with current AI/CPG/recording policy", cpg, enemyOff, wingmenOff, recording)
					}
				}
			}
		}
	}
}

func TestFixedPendingDestructionReachesExistingDeathOwner(t *testing.T) {
	for _, kind := range []byte{0x11, 0x16} {
		s, in := fixedFixture(t, 1)
		e := &s.init.Entities[1]
		e.Kind, e.Pose.Height = kind, 0
		fixedRound(s, 20, 1, 2, 3, 0)
		sequence(t, &in, 255)
		if err := s.Step(in); err != nil || e.Participation != 2 || e.ModelID != 0 || e.Kind != kind {
			t.Fatal("contact must not redispatch or immediately replace actor", kind, err, e)
		}
		visits := 1
		if kind == 0x16 {
			visits = 5 //2->3 then four post-decrement countdown visits.
		}
		for n := 0; n < visits; n++ {
			if _, err := s.AdvanceDeaths(in); err != nil {
				t.Fatal(kind, n, err)
			}
			if n < visits-1 && (e.Kind != kind || e.ModelID != 0) {
				t.Fatal("fixed death replaced early", kind, n, e)
			}
		}
		if e.Kind != 0x1c || e.Participation != 0 || e.ModelID != 1 || s.Results[3].Sequence != 1 || s.Results[3].Kind != 1 {
			t.Fatal("fixed/Support victim failed its actual replacement path", kind, e, s.Results)
		}
		if _, err := s.AdvanceDeaths(in); err != nil || e.Participation != 4 || e.Pose.Height != 0 {
			t.Fatal("replacement did not settle on following actor visit", kind, e, err)
		}
	}
}
