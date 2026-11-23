package combat

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func TestOriginalDynamicSpeedAndGuidanceOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source bytes")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Corrected section1 mapping: original0975:IP -> file d8b0+IP.
	for ip, literal := range map[int]string{
		0x7ed2: "a1e200",                       // spawn reads main E2
		0x8920: "268b848825d1e0",               // guided reads dynamic+6
		0x8ced: "268b848825d1e0",               // cannon reads dynamic+6
		0x8f0d: "268b848825d1e0",               // rocket reads dynamic+6
		0x8983: "26f684020008",                 // stored/effective target fork
		0x89a7: "26a0b90afec88846f2",           // selected-1 -> local target
		0x8b48: "26f7af0a008ac48ae22629870e00", // signed product bits8..23
		0x3ee8: "becc00a5a5a5",                 // late publisher copies only angles
	} {
		want, _ := hex.DecodeString(literal)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source operand0975:%04x changed", ip)
		}
	}
	if !bytes.Equal(b[0x487f0+0x2582:][:80*24], make([]byte, 80*24)) {
		t.Fatal("recognized source dynamic rows no longer cold zero")
	}
}

func TestSpawnAndAccelerationUseSeparateSpeedOwners(t *testing.T) {
	s, in := fixture(t, 0x1e21)
	in.DF, in.PlayerSpeed = 10, 400 // supplied boundary, not a natural witnessed speed
	if err := s.Fire(in); err != nil {
		t.Fatal(err)
	}
	p := &s.Slots[s.LastSlot]
	if p.Speed != 450 {
		t.Fatal("spawn must still use abs(main E2)+50", p.Speed)
	}
	// Independent source comparisons: 450<600 -> +225; then675>=600
	// and675>=2*dynamic0 -> retained675, regardless of changing main E2.
	for _, want := range []int16{675, 675, 675} {
		if move := s.accelerate(p, in); p.Speed != want || move != want {
			t.Fatalf("player dynamic0: speed%d move%d want%d", p.Speed, move, want)
		}
	}
	// A different shooter with dynamic400 really does retain the800 threshold.
	p.Shooter, p.Speed = 1, 450
	s.init.Entities[1].RawMotion = 400
	for _, want := range []int16{675, 1012, 1012} {
		if move := s.accelerate(p, in); p.Speed != want || move != want {
			t.Fatalf("dynamic400: speed%d move%d want%d", p.Speed, move, want)
		}
	}
}

func TestOrdinarySpawnSpeedsRemainUnchanged(t *testing.T) {
	for _, tc := range []struct {
		e2   int16
		want []int16
	}{
		{0, []int16{75, 112, 168, 252, 378, 567, 850, 850}},
		{120, []int16{255, 382, 573, 859, 859}},
		{240, []int16{435, 652, 652}},
		{-75, []int16{187, 280, 420, 630, 630}},
	} {
		s, in := fixture(t, 0x1e21)
		in.DF, in.PlayerSpeed = 10, tc.e2
		if err := s.Fire(in); err != nil {
			t.Fatal(err)
		}
		p := &s.Slots[s.LastSlot]
		for n, want := range tc.want {
			if s.accelerate(p, in) != want || p.Speed != want {
				t.Fatalf("E2%d acceleration%d got%d want%d", tc.e2, n, p.Speed, want)
			}
		}
	}
}

func TestCapturedReverseRocketKeepsObservedTrajectory(t *testing.T) {
	s, in := fixture(t, 0x1e21)
	for i := range s.init.Entities {
		s.init.Entities[i].Participation = 0
	}
	// Original rocket-update-01: accepted M247 shot after its83h->31h
	// birth phase. Logged original DF10 is a witness, not the nativeDF6 policy.
	s.Slots[1] = Projectile{Flags: 0x1e21, Lifetime: 49, Packed: 15, Speed: 125, Parameter: 1,
		Pose: gamestate.InitializedPose{H0: 354920, H1: 345694, Height: 266, Angles: [3]uint16{1258, 0, 0}}}
	in.DF, in.PlayerSpeed = 10, -75
	for n := 0; n < 49; n++ {
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		}
	}
	p := s.Slots[1]
	// Source raw sine1956 at file4bcec gives motion height22,33,50,
	// then46*75. H1 sum187+280+420+46*630=29867; no speed/life tuning.
	if p.Pose.H0 != 354920 || p.Pose.H1 != 375561 || p.Pose.Height != 3821 || p.Speed != 630 || p.Lifetime != 0 {
		t.Fatal("original reverse-shot arithmetic changed", p)
	}
	if len(events(s, "impact"))+len(events(s, "terrain-impact")) != 0 {
		t.Fatal("witnessed lifetime expiry became a collision", s.Events)
	}
}

// This fixture is the source8b2a steering witness, with the preceding movement
// inverted independently using original Q14 table operands. At speed280 and
// yaw65505/pitch-155 its movement is(0,+280,-5). No production output supplies
// the expected coordinates or steering. Evidence step35/run02 packets record
// stored38, selected8/effective7, correction220 and resulting pitch-292.
func capturedHellfire(t *testing.T, flags uint16) (*State, Input) {
	t.Helper()
	s, in := fixture(t, flags)
	for len(s.init.Entities) < 39 {
		s.init.Entities = append(s.init.Entities, gamestate.InitializedEntity{RuntimeRow: len(s.init.Entities), Participation: 1, Kind: 3})
	}
	s.init.Entities[7].Pose = gamestate.InitializedPose{H0: 356448, H1: 342364}
	s.init.Entities[38].Pose = gamestate.InitializedPose{H0: 355356, H1: 350564}
	s.Slots[5] = Projectile{Flags: flags, Lifetime: 126, Packed: 19, Shooter: 0, Target: 38, Speed: 187, Parameter: 30,
		Pose: gamestate.InitializedPose{H0: 354920, H1: 333785, Height: 288, Angles: [3]uint16{65381, 65505, 0}}}
	in.DF, in.PlayerSpeed, in.Target, in.TargetAvailable = 10, -75, 8, true
	return s, in
}

func TestWitnessedHellfireRetargetKeepsStoredIdentity(t *testing.T) {
	s, in := capturedHellfire(t, 0x0e10)
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	p := s.Slots[5]
	if p.Target != 38 || p.Speed != 280 || p.Lifetime != 125 || p.Pose.H0 != 354920 || p.Pose.H1 != 334065 || p.Pose.Height != 283 || p.Pose.Angles != ([3]uint16{65244, 65285, 0}) {
		t.Fatalf("captured steering: got%+v; expected stored38, yaw65505-220, pitch-292", p)
	}
}

func TestStaticGuidanceBranchQualifications(t *testing.T) {
	for _, tc := range []struct {
		name               string
		flags              uint16
		selected           uint16
		available, dead    bool
		parameter          int16
		wantYaw, wantPitch uint16
	}{
		//08 is static source evidence here; Step35 did not witness an MMW shot.
		{"MMW-stored38", 0x0e18, 8, true, false, 30, 65477, 65379},
		{"ordinary-selection-loss", 0x0e10, 0, false, false, 30, 65505, 65381},
		{"ordinary-display-loss", 0x0e10, 8, false, false, 30, 65505, 65381},
		// A targetless launch owns Parameter0; later acquisition cannot restore
		// yaw gain, although its pitch writer still runs. This is not lock proof.
		{"targetless-zero-gain", 0x0e10, 8, true, false, 0, 65505, 65244},
		// Source steering does not check Participation. Keep dead-row aiming
		// separate from damage's Participation==1 guard and future slot reuse.
		{"dead-row-still-steers", 0x0e10, 8, true, true, 30, 65285, 65244},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := capturedHellfire(t, tc.flags)
			in.Target, in.TargetAvailable = tc.selected, tc.available
			s.Slots[5].Parameter = tc.parameter
			stored := byte(38)
			if tc.parameter == 0 {
				stored = 0 // targetless spawn clears both stored target and gain
				s.Slots[5].Target = stored
			}
			if tc.dead {
				s.init.Entities[7].Participation = 0
			}
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
			p := s.Slots[5]
			if p.Target != stored || p.Pose.Angles[1] != tc.wantYaw || p.Pose.Angles[0] != tc.wantPitch {
				t.Fatal("guidance target domains changed", p)
			}
		})
	}
}
