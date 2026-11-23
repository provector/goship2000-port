package combat

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"strings"
	"testing"
)

func attackFixture(t *testing.T) (*State, Input, [16][8]byte) {
	t.Helper()
	s, in := incomingFixture(t, 3)
	s.SetMissionActorsEnabled(true)
	s.init.Entities[0].ControlRaw[1] = 1
	e := &s.init.Entities[1]
	e.Kind, e.Participation = 2, 1
	e.Pose = s.init.Entities[0].Pose
	e.Pose.H1 -= 1024
	e.Pose.Angles[1] = 0x8000 // kind2's source gun-bearing exception
	e.Object.Raw[11] = 0x22
	in.Contacts[1].Flags = 2
	in.E0, in.DF = true, 2
	var defs [16][8]byte
	for i := range defs {
		defs[i][0] = byte(i)
	}
	defs[2] = [8]byte{2, 230, 3, 0, 1, 0, 23, 0} // immutable ordinary gun row2
	if err := s.ConfigureStationaryEnemyAI(defs, false); err != nil {
		t.Fatal(err)
	}
	return s, in, defs
}

func TestStationaryCallerSelectionCooldownAndNaturalGun(t *testing.T) {
	s, in, _ := attackFixture(t)
	draws, targets := 0, 0
	in.Random = func() uint16 { draws++; return 1 }
	in.HostileTarget = func(actor, target int) {
		if actor != 1 || target != 0 {
			t.Fatal(actor, target)
		}
		targets++
	}
	if err := s.advanceGroundRoute(1, in); err != nil {
		t.Fatal(err)
	}
	p := s.Slots[21]
	if p.Flags != 3 || p.Shooter != 1 || p.Target != 0 || p.Lifetime != 131 || p.Speed != 306 || p.Packed != 230 || p.Parameter != 0 || p.Pose.Height != s.init.Entities[1].Pose.Height+32 || draws != 1 || targets != 1 {
		t.Fatal("natural A070 gun constructor", p, draws, targets)
	}
	if s.init.Entities[1].DynamicRaw[3] != 3 || s.init.Entities[1].Pose.Angles[1] != 0 || s.FilmState().Enemy.NextSlot != 21 {
		t.Fatal("source pre-allocation writers", s.init.Entities[1], s.FilmState().Enemy)
	}
	in.E0 = false
	if err := s.advanceGroundRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if draws != 2 || targets != 2 || s.init.Entities[1].DynamicRaw[3] != 3 {
		t.Fatal("waiting decision consumes draw before E0 gate", draws, targets, s.init.Entities[1].DynamicRaw[3])
	}
	in.E0 = true
	if err := s.advanceGroundRoute(1, in); err != nil {
		t.Fatal(err)
	}
	if draws != 3 || s.init.Entities[1].DynamicRaw[3] != 2 {
		t.Fatal(draws, s.init.Entities[1].DynamicRaw[3])
	}
	if len(events(s, "launch")) != 1 || len(events(s, "sound")) != 1 || events(s, "sound")[0].Command != 0x29 {
		t.Fatal(s.Events)
	}
}

func TestStationaryNoTargetObserverAndDisableControls(t *testing.T) {
	for _, tc := range []struct {
		name        string
		flags, kind byte
		control     byte
		want        bool
	}{
		{"ordinary-retaliation", 2, 0xff, 1, true},
		{"untracked", 0, 0x11, 1, false},
		{"inactive-observer", 2, 0xff, 0, false},
		{"mast-token-excluded", 0xfa, 0xff, 1, false},
		{"kind11-alternative", 2, 0x11, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in, _ := attackFixture(t)
			s.init.Entities[0].ControlRaw[1] = tc.control
			in.Contacts[1].Flags, in.Contacts[1].Kind, in.Contacts[1].AttackFlags = tc.flags, tc.kind, 0xffff
			draws := 0
			in.Random = func() uint16 { draws++; return 1 }
			got, err := s.stationaryEnemyAttack(1, in)
			if err != nil || got != tc.want || draws != map[bool]int{true: 1, false: 0}[tc.want] {
				t.Fatal(got, err, draws)
			}
			if !tc.want && in.Contacts[1].AttackFlags == 0 {
				// Value-copy inputs do not publish to this copy. The caller's
				// real CONTACT owner is covered separately below.
				t.Fatal("unexpected mutation of copied diagnostic input")
			}
		})
	}
	s, in, _ := attackFixture(t)
	contacts := in.Contacts
	contacts[1].AttackFlags, contacts[1].Flags = 0xffff, 0
	in.ContactState = &contacts
	if got, err := s.stationaryEnemyAttack(1, in); err != nil || got || contacts[1].AttackFlags != 0 {
		t.Fatal(got, err, contacts[1])
	}
	s.enemyAutonomyDisabled = true
	draws := 0
	in.Random = func() uint16 { draws++; return 1 }
	before, _ := json.Marshal(s.FilmState())
	if got, err := s.stationaryEnemyAttack(1, in); err != nil || got || draws != 0 {
		t.Fatal(got, err, draws)
	}
	after, _ := json.Marshal(s.FilmState())
	if !reflect.DeepEqual(before, after) {
		t.Fatal("disabled decision changed mechanics")
	}
}

func TestStationaryRangeConeBusyAndQualityTurn(t *testing.T) {
	for _, tc := range []struct {
		flags uint16
		yaw   uint16
		want  bool
	}{
		{1, 0x3ff, true}, {1, 0x400, false}, {0x80, 0x7ff, true}, {0x80, 0x800, false},
	} {
		s, in, _ := attackFixture(t)
		s.init.Entities[1].Kind, s.init.Entities[1].Pose.Angles[1] = 3, tc.yaw
		s.enemyDefinitions[2][4], s.enemyDefinitions[2][5] = byte(tc.flags), byte(tc.flags>>8)
		if _, err := s.stationaryEnemyAttack(1, in); err != nil {
			t.Fatal(err)
		}
		if (len(events(s, "launch")) != 0) != tc.want {
			t.Fatal("strict source cone", tc, s.Events)
		}
	}
	for _, quality := range []uint16{0, 1, 2, 3} {
		s, in, _ := attackFixture(t)
		e := &s.init.Entities[1]
		e.Kind, e.Pose.Angles[1] = 3, 0x1000
		s.init.Switches = quality
		if got, err := s.stationaryEnemyAttack(1, in); err != nil || !got {
			t.Fatal(got, err)
		}
		want := uint16(0x1000 - int16(0x1000)/[4]int16{50, 20, 10, 5}[quality])
		if e.Pose.Angles[1] != want || len(events(s, "launch")) != 0 {
			t.Fatal(quality, e.Pose.Angles[1], want)
		}
	}
	for _, tc := range []struct {
		name     string
		distance uint32
		want     bool
	}{{"range-inside", 23*32*16 - 16, true}, {"range-equal", 23 * 32 * 16, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, in, _ := attackFixture(t)
			s.init.Entities[1].Pose.H1 = s.init.Entities[0].Pose.H1 - tc.distance
			if _, err := s.stationaryEnemyAttack(1, in); err != nil {
				t.Fatal(err)
			}
			if (len(events(s, "launch")) > 0) != tc.want {
				t.Fatal(s.Events)
			}
		})
	}
	s, in, _ := attackFixture(t)
	s.Slots[21].Lifetime = 1
	s.Slots[20].Lifetime = 0
	if _, err := s.stationaryEnemyAttack(1, in); err != nil {
		t.Fatal(err)
	}
	if len(events(s, "launch")) != 0 || len(events(s, "hostile-busy")) != 1 || s.stationaryEnemy.NextSlot != 21 || s.init.Entities[1].DynamicRaw[3] != 3 || s.init.Entities[1].Pose.Angles[1] != 0 {
		t.Fatal("busy ring must not search or undo earlier writers", s.Events, s.stationaryEnemy, s.init.Entities[1])
	}
}

func TestGuidedConstructorAndGunAcceptedRejectedHit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		r      uint16
		bypass bool
	}{{"equal", 90, false}, {"above", 91, true}} {
		t.Run(tc.name, func(t *testing.T) {
			s, in, _ := attackFixture(t)
			s.enemyDefinitions[2] = [8]byte{2, 128, 24, 90, 0x80, 0, 100, 0}
			draws := 0
			in.Random = func() uint16 {
				draws++
				if draws == 1 {
					return 1
				}
				return tc.r
			}
			if _, err := s.stationaryEnemyAttack(1, in); err != nil {
				t.Fatal(err)
			}
			p := s.Slots[21]
			if draws != 2 || p.Parameter != 128 || p.Packed != 0 || p.Speed != 102 || (p.Flags&0x8000 != 0) != tc.bypass || p.Template != 1 {
				t.Fatal(p, draws)
			}
		})
	}
	for _, accepted := range []bool{false, true} {
		s, in, _ := attackFixture(t)
		hits, draws := 0, 0
		in.Random = func() uint16 {
			draws++
			if draws == 1 {
				return 1
			}
			if accepted {
				return 231
			}
			return 230
		}
		in.Incoming.ApplyPlayerHit = func(slot int) (uint16, error) { hits++; return 2, nil }
		if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
			t.Fatal(err)
		}
		for tick := 0; tick < 20 && s.Slots[21].Lifetime != 0; tick++ {
			if err := s.Step(in); err != nil {
				t.Fatal(tick, err)
			}
		}
		if s.Slots[21].Lifetime != 0 || hits != map[bool]int{true: 1, false: 0}[accepted] || draws != 2 {
			t.Fatal(accepted, s.Slots[21], hits, draws)
		}
		if accepted && len(events(s, "hostile-impact")) != 1 || !accepted && len(events(s, "hostile-miss")) != 1 {
			t.Fatal("actual terminal branch needs bounded evidence", accepted, s.Events)
		}
	}
}

func TestNaturalNonplayerTrajectoryRejectAndAcceptedHitGuard(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		s, in, _ := attackFixture(t)
		target, shooter := &s.init.Entities[1], &s.init.Entities[2]
		target.Kind, target.Participation, target.ControlRaw[1] = 0x20, 1, 2
		shooter.Kind, shooter.Participation, shooter.DynamicRaw[2], shooter.Object.Raw[11] = 2, 1, 0, 0x22
		shooter.Pose = target.Pose
		shooter.Pose.H1 -= 1024
		in.Contacts[2].Flags = 2 | 1<<3
		in.Random = func() uint16 { return 255 }
		if !accepted {
			s.enemyDefinitions[2][1] = 255
		}
		playerHits := 0
		in.Incoming.ApplyPlayerHit = func(int) (uint16, error) { playerHits++; return 1, nil }
		if got, err := s.stationaryEnemyAttack(2, in); err != nil || !got {
			t.Fatal(got, err)
		}
		if s.Slots[21].Target != 1 {
			t.Fatal("wrong recipient", s.Slots[21])
		}
		var impactErr error
		for tick := 0; tick < 20 && s.Slots[21].Lifetime != 0; tick++ {
			impactErr = s.Step(in)
			if impactErr != nil {
				break
			}
		}
		if accepted && (impactErr == nil || !strings.Contains(impactErr.Error(), "NPC defense/damage consumer94C8..95FD")) || !accepted && impactErr != nil {
			t.Fatal(accepted, impactErr)
		}
		if playerHits != 0 || s.IncomingThreat != 0 || len(events(s, "shake")) != 0 || s.Slots[21].Lifetime != 0 {
			t.Fatal("NPC recipient leaked player effects", playerHits, s.IncomingThreat, s.Slots[21], s.Events)
		}
	}
}

func TestNonplayerGuidanceUsesActualControlParentAndNoPlayerWarning(t *testing.T) {
	for _, followsPlayer := range []bool{false, true} {
		s, in, _ := attackFixture(t)
		target, shooter := &s.init.Entities[1], &s.init.Entities[2]
		target.Kind, target.Participation, target.ControlRaw[1] = 0x20, 1, 2
		if followsPlayer {
			target.ControlRaw[1] = 1
		}
		target.ControlRaw[5] = 0
		shooter.Kind, shooter.Participation, shooter.DynamicRaw[2], shooter.Object.Raw[11] = 2, 1, 0, 0x22
		shooter.Pose = target.Pose
		shooter.Pose.H1 -= 1024
		in.Contacts[2].Flags = 2 | 1<<3
		s.enemyDefinitions[2] = [8]byte{2, 128, 24, 255, 0x80, 0, 100, 0}
		in.Random = func() uint16 { return 1 }
		in.Incoming.Defenses.FlareTimer = 1
		in.Incoming.Defenses.Decoy.H0 = 0x50000
		in.Incoming.Defenses.Decoy.H1 = 0x50000
		if _, err := s.stationaryEnemyAttack(2, in); err != nil {
			t.Fatal(err)
		}
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
		wantH0 := target.Pose.H0
		if followsPlayer {
			wantH0 = in.Incoming.Defenses.Decoy.H0
		}
		if !s.incomingAimKnown || s.incomingAimPose.H0 != wantH0 || s.IncomingThreat != 0 || !s.incomingWarning[21] {
			t.Fatal(followsPlayer, s.incomingAimPose, s.IncomingThreat, s.incomingWarning[21])
		}
		for _, event := range events(s, "sound") {
			if event.Command >= 0x35 && event.Command <= 0x38 {
				t.Fatal("NPC received player incoming warning", event)
			}
		}
	}
}

func TestStationaryDoesNotEnableMovingOrAircraftCallers(t *testing.T) {
	s, in, _ := attackFixture(t)
	e := &s.init.Entities[1]
	e.DynamicRaw[2] = 0x10
	if _, err := s.stationaryEnemyAttack(1, in); err == nil {
		t.Fatal("moving actor admitted")
	}
	e.DynamicRaw[2], e.Kind = 0, 1
	if _, err := s.stationaryEnemyAttack(1, in); err == nil {
		t.Fatal("aircraft admitted")
	}
}

func TestHostileOriginalOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[int]string{
		0xa08e: "26f687420602", // actual contact-tracked prerequisite
		0xa221: "9a38207f03",   // shared selection RNG before cooldown
		0xa239: "250f00",       // low attack nibble, distinct from high
		0xa2cf: "26fe8f8525",   // byte cooldown decrement
		0xa43c: "26ff06cb0a",   // ACB advances before occupancy
		0xa4e7: "26c687040083", // lifetime131
		0xa50f: "26f6400401",   // gun split uses raw definition bit0
		0xa5b8: "26384503",     // unsigned bypass byte comparison
		0xc056: "9a102cbb10",   // stationary caller
	} {
		p, _ := hex.DecodeString(want)
		if !bytes.Equal(b[0xd8b0+ip:][:len(p)], p) {
			t.Fatalf("original operand%04x differs", ip)
		}
	}
	if b[0x487f0+0xacb] != 20 || b[0x487f0+0xacc] != 0 {
		t.Fatal("source cold ACB ring is not20")
	}
}

func TestDisabledAutonomyLeavesLiveProjectileConsumersActive(t *testing.T) {
	s, in, _ := attackFixture(t)
	in.Random = func() uint16 { return 255 }
	if _, err := s.stationaryEnemyAttack(1, in); err != nil {
		t.Fatal(err)
	}
	s.enemyAutonomyDisabled = true
	hits := 0
	in.Incoming.ApplyPlayerHit = func(int) (uint16, error) { hits++; return 1, nil }
	for tick := 0; tick < 20 && s.Slots[21].Lifetime != 0; tick++ {
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 1 || s.Slots[21].Lifetime != 0 {
		t.Fatal(hits, s.Slots[21])
	}
}

func TestActualAscendingActorsRingAndDefinitionNibble(t *testing.T) {
	s, in, _ := attackFixture(t)
	for actor := 1; actor <= 2; actor++ {
		e := &s.init.Entities[actor]
		e.Kind, e.Participation, e.DynamicRaw[2], e.Object.Raw[11] = 2, 1, 0, 0x23
		e.Pose = s.init.Entities[1].Pose
		in.Contacts[actor].Flags = 2
	}
	s.enemyDefinitions[3] = s.enemyDefinitions[2]
	s.enemyDefinitions[3][0] = 3
	values := []uint16{0, 1}
	at := 0
	in.Random = func() uint16 { v := values[at]; at++; return v }
	if _, err := s.advanceLogisticsFollowers(in, nil); err != nil {
		t.Fatal(err)
	}
	launched := events(s, "launch")
	if len(launched) != 2 || launched[0].Actor != 1 || launched[0].Value != 2 || launched[0].Slot != 21 || launched[1].Actor != 2 || launched[1].Value != 3 || launched[1].Slot != 22 || at != 2 {
		t.Fatal("ascending actors/high then low nibble/shared draw ownership", launched, at)
	}
	s.stationaryEnemy.NextSlot = 23
	s.init.Entities[1].DynamicRaw[3] = 0
	in.Random = func() uint16 { return 0 }
	if _, err := s.stationaryEnemyAttack(1, in); err != nil {
		t.Fatal(err)
	}
	if s.stationaryEnemy.NextSlot != 20 || s.Slots[20].Lifetime != 131 {
		t.Fatal("ring wrap", s.stationaryEnemy, s.Slots[20])
	}
}

func TestGunCueUsesSourceDEGateNotSpeechBit(t *testing.T) {
	for _, tc := range []struct {
		audio, speech bool
		cues          int
	}{
		{false, false, 0}, {false, true, 0}, {true, false, 1}, {true, true, 1},
	} {
		s, in, _ := attackFixture(t)
		in.AudioEnabled, in.SpeechEnabled = tc.audio, tc.speech
		draws := 0
		in.Random = func() uint16 { draws++; return 1 }
		if _, err := s.stationaryEnemyAttack(1, in); err != nil {
			t.Fatal(err)
		}
		if len(events(s, "sound")) != tc.cues || draws != 1 || s.Slots[21].Lifetime != 131 {
			t.Fatal("A516 DE!=0 is independent of DE80 speech and does not change constructor/RNG", tc, s.Events, draws)
		}
		if tc.cues != 0 && events(s, "sound")[0].Command != 0x29 {
			t.Fatal(s.Events)
		}
	}
}

func TestIncomingOutcomeDiagnosticsStayOutOfCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		name        string
		yaw, height uint16
		life        byte
		reason      string
	}{
		{"behind", 0x8000, 512, 10, "97E4/97E9"},
		{"terrain", 0, 0xfffe, 10, "9933/993D"},
		{"expired", 0, 512, 1, "lifetime decrement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in, _ := attackFixture(t)
			p := incomingAtTarget(s, 3, 255)
			p.Pose.H1 = s.init.Entities[0].Pose.H1 - 1024
			p.Pose.Angles[1], p.Pose.Height, p.Lifetime = tc.yaw, tc.height, tc.life
			s.Slots[20], s.incomingAdmitted[20] = p, true
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
			misses := events(s, "hostile-miss")
			if len(misses) != 1 || !strings.Contains(misses[0].Source, tc.reason) || misses[0].Actor != 1 || misses[0].Target != 0 || s.Slots[20].Lifetime != 0 {
				t.Fatal(tc.name, misses, s.Slots[20])
			}
			before := s.FilmState()
			s.incomingOutcome("hostile-miss", 20, 1, "diagnostic checkpoint exclusion control")
			if !reflect.DeepEqual(before, s.FilmState()) {
				t.Fatal("diagnostic changed checkpoint")
			}
			// A historical interpretation has neither these operational events
			// nor a new optional checkpoint owner.
			s.stationaryEnemy = nil
			count := len(s.Events)
			s.incomingOutcome("hostile-miss", 20, 1, "historical gate")
			if len(s.Events) != count || s.FilmState().Enemy != nil {
				t.Fatal("historical diagnostic gate failed")
			}
		})
	}
}
