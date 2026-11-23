package combat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func aircraftFixture(t *testing.T, kind byte, grouped byte) (*State, Input, *int) {
	t.Helper()
	s, in := groundFixture(t)
	s.enemyBehaviorEnabled = true
	var definitions [16][8]byte
	for i := range definitions {
		definitions[i][0] = byte(i)
	}
	if err := s.ConfigureStationaryEnemyAI(definitions, false); err != nil {
		t.Fatal(err)
	}
	s.init.Offsets[1] = [][2]int16{{100, 0}, {200, 0}, {300, 0}}
	e := &s.init.Entities[10]
	e.Kind, e.Participation, e.DynamicRaw[2], e.RawMotion = kind, 1, grouped, 100
	e.Pose = gamestate.InitializedPose{H0: 262144, H1: 262144, Height: 1024}
	e.Object.Raw[10], e.Object.Raw[11] = kind, 0 // canonical unarmed choice still returns selected.
	player := &s.init.Entities[s.init.PlayerRow]
	player.Pose = gamestate.InitializedPose{H0: 262144, H1: 278144, Height: 512}
	player.ControlRaw[1] = 1
	in.Contacts[10].Flags = 2 // actual observer0, independent of player's weapon target.
	in.DF, in.E0 = 1, true
	draws := new(int)
	in.Random = func() uint16 { *draws++; return 4 }
	return s, in, draws
}

func TestAircraftCallerOriginalOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != "1a1db96070adb8c23e1eb499b449cc147f9fb0fd7b47a65e88c4c5603a40e209" {
		t.Fatal("unsupported original fixture")
	}
	for ip, raw := range map[int]string{
		0xb3b5: "9a102cbb10", 0xb3c9: "26f684842501", 0xb3d4: "26c684842500",
		0xb5ce: "9a102cbb10", 0xb614: "f7e9b90a0099f7f9",
		0xb672: "268a8f4206", 0xb677: "c0e903", 0xb688: "d1f903c1058000",
		0xb6fa: "26c78786250000", 0xb701: "26c687832501", 0xb707: "26c78798250004",
		0xb70e: "26c78788259001", 0xb82e: "9a102cbb10", 0xb836: "8946d2",
		0xb9ab: "837ed200", 0xba07: "f7e9b90a0099f7f9", 0xba29: "9a6c0c7f03",
	} {
		want, err := hex.DecodeString(raw)
		if err != nil || !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source0975:%04x operand mismatch", ip)
		}
	}
}

func TestHelicopterGroupedSelectedWithoutShotDetachesOddOnly(t *testing.T) {
	for _, code := range []byte{0x11, 0x12} {
		t.Run(hex.EncodeToString([]byte{code}), func(t *testing.T) {
			s, in, draws := aircraftFixture(t, 0, code)
			e := &s.init.Entities[10]
			s.init.Entities[9].DynamicRaw[2] = 0x81
			before := e.Pose
			if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
				t.Fatal(err)
			}
			if len(events(s, "launch")) != 0 || s.init.Entities[9].DynamicRaw[2] != 1 {
				t.Fatal("zero definition released or failed to clear PATH marker", s.Events)
			}
			if code&1 != 0 {
				if e.DynamicRaw[2] != 0 || e.Pose != before || *draws != 1 {
					t.Fatal("odd selected member must detach without movement or jitter", e, before, *draws)
				}
				// The following visit is genuinely independent, not another
				// group movement or a permanent stopped actor.
				if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
					t.Fatal(err)
				}
				if e.Pose.H1 != before.H1+10 || *draws != 2 {
					t.Fatal("detached successor did not integrate B5CE", e.Pose, *draws)
				}
			} else if e.DynamicRaw[2] != code || *draws != 3 || e.Pose.H0 != before.H0+75 {
				t.Fatal("even selected member must retain source formation/two jitter draws", e, *draws)
			}
		})
	}
}

func TestHelicopterOddDetachAcrossDecisionOutcomes(t *testing.T) {
	for _, outcome := range []string{"unarmed", "cooldown", "busy", "release"} {
		t.Run(outcome, func(t *testing.T) {
			s, in, draws := aircraftFixture(t, 0, 0x11)
			e := &s.init.Entities[10]
			s.init.Entities[0].Pose.H1 = e.Pose.H1 + 1024
			s.enemyDefinitions[2] = [8]byte{2, 230, 3, 0, 1, 0, 23, 0}
			e.Object.Raw[11] = 0x22
			switch outcome {
			case "unarmed":
				e.Object.Raw[11] = 0
			case "cooldown":
				e.DynamicRaw[3] = 2
			case "busy":
				s.Slots[21].Lifetime = 2
			}
			before := e.Pose
			if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
				t.Fatal(err)
			}
			if e.DynamicRaw[2] != 0 || e.Pose.H0 != before.H0 || e.Pose.H1 != before.H1 || *draws != 1 {
				t.Fatal("selected outcome did not detach/skip once", outcome, e, *draws)
			}
			if (len(events(s, "launch")) == 1) != (outcome == "release") || (len(events(s, "hostile-busy")) == 1) != (outcome == "busy") {
				t.Fatal("caller confused target selection and release", outcome, s.Events)
			}
		})
	}
}

func TestHelicopterDetachDoesNotReadUnreachedOffsets(t *testing.T) {
	s, in, draws := aircraftFixture(t, 0, 0x11)
	s.init.Offsets[1] = nil // B3DD is deliberately not reached by this fixture.
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal("selected odd arm must not read formation table", err)
	}
	if s.init.Entities[10].DynamicRaw[2] != 0 || *draws != 1 {
		t.Fatal("detach order", s.init.Entities[10], *draws)
	}
	// No target reaches B3DD and still requires actual offset provenance.
	s, in, _ = aircraftFixture(t, 0, 0x11)
	s.init.Offsets[1] = nil
	in.Contacts[10].Flags = 0
	if err := s.advanceOrdinaryHelicopter(10, in); err == nil {
		t.Fatal("unrecovered formation span was silently guessed")
	}
}

func TestHelicopterIndependentMovementHeightAndDisabledControl(t *testing.T) {
	for quality := uint16(0); quality < 4; quality++ {
		s, in, draws := aircraftFixture(t, 0, 0)
		s.init.Switches = quality
		e := &s.init.Entities[10]
		before := e.Pose
		if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
			t.Fatal(err)
		}
		h, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(before))
		if err != nil {
			t.Fatal(err)
		}
		if e.Pose.H0 != before.H0 || e.Pose.H1 != before.H1+10 || e.Pose.Height != uint16(h+256+128) || *draws != 1 {
			t.Fatal("selected no-shot solo integration/observer-height owner", quality, e.Pose, h, *draws)
		}
		// No acquisition is invented when the frozen AI policy disables it.
		s.enemyAutonomyDisabled = true
		e.Pose.Angles[2] = 0xff00
		before = e.Pose
		if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
			t.Fatal(err)
		}
		before.Angles[2] = 0xff80
		if e.Pose != before || *draws != 1 {
			t.Fatal("disabled solo must follow no-target SAR roll", e.Pose, before, *draws)
		}
	}
}

func TestHelicopterContactLossAndQueuedWaypointOwnTheirVisits(t *testing.T) {
	s, in, draws := aircraftFixture(t, 0, 0)
	e := &s.init.Entities[10]
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	in.Contacts[10].Flags = 0 // no fallback-eligible fixed actor in this fixture.
	e.Pose.Angles[2] = 0x0200
	before := e.Pose
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	before.Angles[2] = 0x0100
	if e.Pose != before || *draws != 1 {
		t.Fatal("lost contact must return to damping-only idle without a draw", e.Pose, before, *draws)
	}
	in.Contacts[10].Flags = 2
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	if *draws != 2 || e.Pose.H1 == before.H1 {
		t.Fatal("fresh contact did not resume source pursuit visit", e.Pose, *draws)
	}
	// An already queued private/formation waypoint is checked before B393;
	// it must not acquire/fire, even with the same eligible contact available.
	s.route.Queued[10] = 0
	e.LeaderRow = 9
	e.Pose.H0 += 200
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	if *draws != 2 || len(events(s, "launch")) != 0 {
		t.Fatal("queued source route leaked into hostile decision", *draws, s.Events)
	}
}

func TestHelicopterHeightRetainsContactObserverAfterFallback(t *testing.T) {
	s, in, draws := aircraftFixture(t, 0, 0)
	observer := &s.init.Entities[1]
	observer.ControlRaw[1], observer.Pose.Height = 0, 600
	fixed := &s.init.Entities[2]
	fixed.Kind, fixed.Participation = 0x17, 1
	fixed.Pose = gamestate.InitializedPose{H0: 262144, H1: 266144, Height: 2000}
	binary.LittleEndian.PutUint16(fixed.Object.Raw[27:], 0xffff)
	in.Contacts[10].Flags = 2 | 1<<3
	selected := -1
	in.HostileTarget = func(actor, target int) { selected = target }
	e := &s.init.Entities[10]
	before := e.Pose
	if err := s.advanceOrdinaryHelicopter(10, in); err != nil {
		t.Fatal(err)
	}
	h, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(before))
	if err != nil {
		t.Fatal(err)
	}
	if selected != 2 || e.Pose.Height != uint16(h+300+128) || *draws != 1 {
		t.Fatal("B672 height must use CONTACT observer1, not A070 fallback2", selected, e.Pose, h, *draws)
	}
}

func TestFixedWingColdVisitAndSelectedSteeringOrder(t *testing.T) {
	s, in, draws := aircraftFixture(t, 1, 0)
	e := &s.init.Entities[10]
	e.Participation, e.Pose.Height, e.RawMotion = 0, 0, 0
	e.DynamicRaw[4] = 7
	s.route.Cursor[10] = 99
	before := e.Pose
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Participation != 1 || e.Pose.H0 != before.H0 || e.Pose.H1 != before.H1 || e.Pose.Height != 1024 || e.RawMotion != 400 || binary.LittleEndian.Uint16(e.DynamicRaw[4:]) != 0 || s.route.Cursor[10] != 0 || *draws != 0 {
		t.Fatal("cold B6F2 visit must initialize only", e, *draws)
	}
	// A070 selected true on definition0 must suppress generic B9B4, even
	// though there is no payload. Yaw4000 is strict no-turn equality.
	e.Pose.Angles[1] = 0x4000
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.Angles[1] != 0x4000 || e.Pose.Angles[2] != 0 || e.DynamicRaw[4] != 0 || *draws != 1 {
		t.Fatal("selected fixed-wing generic steering was incorrectly repeated", e, *draws)
	}
	// The independent turn/latch arm precedes the selected-result test.
	e.Pose.Angles[1] = 0x8000
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(e.DynamicRaw[4:]) != 1 || *draws != 2 {
		t.Fatal("A070 true must not suppress turn/home latch", e, *draws)
	}
	// A latched visit does not consult A070 or consume a selection draw.
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if *draws != 2 {
		t.Fatal("latched return consumed hostile decision draw", *draws)
	}
}

func TestAirborneQualitySteeringRunsOnceBeforeCallerMovement(t *testing.T) {
	for quality := uint16(0); quality < 4; quality++ {
		for _, kind := range []byte{0, 1} {
			s, in, draws := aircraftFixture(t, kind, 0)
			s.init.Switches = quality
			e := &s.init.Entities[10]
			e.Pose.Angles[1] = 0x4000
			e.Object.Raw[11] = 0x11 // source row1 range0: selected, never releases.
			if kind == 1 {
				s.init.Entities[0].Pose.H1 = e.Pose.H1 + 32000
			}
			var err error
			if kind == 0 {
				err = s.advanceOrdinaryHelicopter(10, in)
			} else {
				err = s.advanceOrdinaryAircraft(10, in)
			}
			if err != nil {
				t.Fatal(err)
			}
			if *draws != 1 || len(events(s, "launch")) != 0 {
				t.Fatal("quality range-rejected decision draw/release", quality, kind, *draws, s.Events)
			}
			if kind == 0 {
				want := [4]uint16{0x3000, 0x3000, 0x2000, 0x2000}[quality]
				if e.Pose.Angles[1] != want || e.RawMotion != 82 {
					t.Fatal("A726/A739 quality cyclic/SAR speed", quality, e.Pose, e.RawMotion)
				}
			} else {
				want := [4]uint16{0x3f00, 0x3e00, 0x3c00, 0x3800}[quality]
				if e.Pose.Angles[1] != want || e.Pose.Angles[2] != 0x2000 {
					t.Fatal("A776 fixed-wing quality steering repeated/suppressed", quality, e.Pose)
				}
			}
		}
	}
}

func TestAircraftHistoricalGateAndLowWordMovement(t *testing.T) {
	s, in, draws := aircraftFixture(t, 1, 0)
	s.enemyBehaviorEnabled = false
	e := &s.init.Entities[10]
	e.Pose.Angles[1] = 0x4000
	if err := s.advanceOrdinaryAircraft(10, in); err != nil {
		t.Fatal(err)
	}
	if e.Pose.Angles[1] != 0x3c00 || e.Pose.Angles[2] != 0x1000 || *draws != 0 {
		t.Fatal("historical no-A070 interpretation changed", e, *draws)
	}
	// Source IMUL's upper word is discarded by CWD before IDIV10.
	for _, tc := range []struct {
		speed int16
		df    byte
		step  int16
	}{{10000, 7, 446}, {10000, 6, -553}, {400, 1, 40}} {
		s, in, _ := aircraftFixture(t, 1, 0)
		s.enemyAutonomyDisabled = true
		in.DF = tc.df
		e := &s.init.Entities[10]
		e.RawMotion = tc.speed
		s.init.Entities[s.init.PlayerRow].Pose.Height = 896 // zero desired pitch.
		before := e.Pose
		if err := s.advanceOrdinaryAircraft(10, in); err != nil {
			t.Fatal(err)
		}
		if int32(e.Pose.H1-before.H1) != int32(tc.step) {
			t.Fatal("low-word signed movement", tc, before, e.Pose)
		}
	}
}
