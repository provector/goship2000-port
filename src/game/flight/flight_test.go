package flight

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func sourceFixture(t *testing.T) (Parameters, Queries) {
	t.Helper()
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional source bytes absent")
	}
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.ReadFile(testfixture.Path("installation/APA_STAT.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	var p Parameters
	copy(p.Raw[:], stat)
	b := &world.Bundle{Sine: make([]int16, 1025), AngleBases: make([]uint16, 8), Atan: make([]uint16, 4096)}
	for i := range b.Sine {
		b.Sine[i] = int16(binary.LittleEndian.Uint16(raw[0x4bc50+2*i:]))
	}
	for i := range b.AngleBases {
		b.AngleBases[i] = binary.LittleEndian.Uint16(raw[0x4c452+2*i:])
	}
	for i := range b.Atan {
		b.Atan[i] = binary.LittleEndian.Uint16(raw[0x4c462+2*i:])
	}
	return p, Queries{Angles: b, Height: func(Pose) (int16, error) { return 0, nil }, Cell: func(Pose) (byte, error) { return 0, nil }}
}

func TestOriginalPlayerOperandsAndStat(t *testing.T) {
	p, _ := sourceFixture(t)
	if p.Raw[0x40] != 10 || binary.LittleEndian.Uint16(p.Raw[0x46:]) != 22 || binary.LittleEndian.Uint16(p.Raw[62:]) != 1765 {
		t.Fatal("source APA STAT inputs changed")
	}
	b, _ := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	for ip, literal := range map[int]string{
		0x26de: "f6063d5902",           // SETUP-derived control carrier593d, not mission3be4
		0x2ce1: "f6063d5902",           // same carrier governs hover manual-axis cancellation
		0x28a4: "26833eff0a00",         // environmental disturbance disables hold
		0x26f4: "a0df002ae4",           // unsigned elapsed byte
		0x2730: "f72e3613d1e0",         // low IMUL then SHL1, not ndisasm's historical0
		0x2844: "c1f80533c22bc2",       // prior CWD sign reused after SAR5
		0x2fe9: "268b879600",           // STAT table read
		0x30f4: "268b879600",           // autorotation uses same STAT table
		0x31e4: "268b0e3a06c1f9032bc1", // source runtime mass term, not SHHD
		0x333e: "0306e600d1f8a3e600",   // source lateral mean
		0x33c2: "3d7d007e09",           // easy-forward maximum125
		0x3509: "8a0edf002aed",         // unsigned DF before history integration
		0x3576: "f7e98bd1b90a00",       // low product before CWD/IDIV10
		0x35fe: "6a30ff36ce00",         // four rotated48 footprint probes
		0x3bbd: "26a37604",             // late instrument writer is indispensable
	} {
		want, e := hex.DecodeString(literal)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("source section1 original0975:%04x changed", ip)
		}
	}
}

func TestEnvironmentalDisturbanceDisablesHeightHoldBeforeContact(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct {
		disturbance  int16
		hold, height int16
	}{{0, 1, 800}, {1, 0, 400}} {
		s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 400}, RadarAltitude: 398, Rotor: 1, HeightControl: 1, HeightTarget: 800}
		if e := Step(&s, p, Input{DF: 6, Phase: 1, AutoPilotDisturbance: tc.disturbance}, q); e != nil {
			t.Fatal(e)
		}
		if s.HeightControl != tc.hold || s.Pose.Height != tc.height {
			t.Fatalf("AFF%d sourcehold/height got%d/%d want%d/%d", tc.disturbance, s.HeightControl, s.Pose.Height, tc.hold, tc.height)
		}
	}
}

func TestControlModeDoesNotAliasMissionSwitches(t *testing.T) {
	p, q := sourceFixture(t)
	base := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300}, RadarAltitude: 298, Forward: 64, IndicatedSpeed: 64, Rotor: 1, Collective: 16}
	for _, flags := range []byte{0, 2} {
		var results []State
		for _, switches := range []uint16{0, 2} {
			s := base
			if err := Step(&s, p, Input{DF: 6, Phase: 1, X: 16, ControlFlags: flags, Switches: switches}, q); err != nil {
				t.Fatal(err)
			}
			results = append(results, s)
		}
		if !reflect.DeepEqual(results[0], results[1]) {
			t.Fatal("mission bit02 changed SETUP control-mode response")
		}
		want := uint16(0xffb8) // source27b3: low16(6*16)/−10 *STAT4e(8)=−72
		if flags&2 != 0 {
			want = 0xffdc
		} // source26f4: −abs(64SAR4)*(96/10)=−36
		if results[0].Pose.Angles[1] != want {
			t.Fatalf("control mode%02x heading%04x want%04x", flags, results[0].Pose.Angles[1], want)
		}
	}
	for _, flags := range []byte{0, 2} {
		s := base
		s.Hover = 1
		s.Forward = 0
		if err := Step(&s, p, Input{DF: 6, Phase: 1, X: 64, ControlFlags: flags, Switches: 2}, q); err != nil {
			t.Fatal(err)
		}
		want := int16(1)
		if flags&2 != 0 {
			want = 0
		}
		if s.Hover != want {
			t.Fatalf("hover control carrier flags%02x got%d", flags, s.Hover)
		}
	}
}

func TestRealisticMultiStepHandTrace(t *testing.T) {
	p, q := sourceFixture(t)
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300}, RadarAltitude: 298, Rotor: 1, Collective: 31}
	// Actual APA table[31]=1765. First thrust882, Vtarget=(882/44)/2=10,
	// vertical approaches0→2; height +(2+0)*10/10 =2. Second thrust1323,
	// Vtarget15, vertical2→4; height +(4+1)*6/10 =3. No E0 on third update.
	want := [][4]int16{{882, 2, 302, 300}, {1323, 4, 305, 303}, {1323, 4, 308, 306}}
	for i, v := range want {
		in := Input{DF: 6, E0: i < 2, Phase: uint16(i + 1), Switches: 4}
		if i == 0 {
			in.DF = 10
		}
		if err := Step(&s, p, in, q); err != nil {
			t.Fatal(err)
		}
		got := [4]int16{s.Thrust, s.Vertical, s.Pose.Height, s.RadarAltitude}
		if got != v {
			t.Fatalf("step%d got%v want%v", i, got, v)
		}
		if s.Pose.H0 != 200000 || s.Pose.H1 != 200000 || s.IndicatedSpeed != 0 {
			t.Fatal("zero horizontal force/display branch changed")
		}
	}
}

func TestEasyMotionHasDifferentSourceLiftAndInstrumentWriter(t *testing.T) {
	p, q := sourceFixture(t)
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300}, RadarAltitude: 298, Rotor: 1, Collective: 31}
	// The easy branch subtracts STAT[0x30]=1486, not the realistic mass term.
	// First target(882−1486)SAR5=−19; vertical0→−2. SAR(−2,2)=−1,
	// so the height term (−2−1)*10/10=−3 gives300→297.
	if err := Step(&s, p, Input{DF: 10, E0: true, Phase: 1}, q); err != nil {
		t.Fatal(err)
	}
	if s.Vertical != -2 || s.Pose.Height != 297 {
		t.Fatalf("easy source branch %+v", s)
	}
	s.Forward, s.Vertical = 100, -10
	if err := Step(&s, p, Input{DF: 6, E0: false, Phase: 2}, q); err != nil {
		t.Fatal(err)
	}
	if s.IndicatedSpeed != 110 {
		t.Fatalf("late0476 expected E2+absE4, got%d", s.IndicatedSpeed)
	}
	s.Forward = -100
	if err := Step(&s, p, Input{DF: 6, E0: false, Phase: 3}, q); err != nil {
		t.Fatal(err)
	}
	if s.IndicatedSpeed != -110 {
		t.Fatalf("negative late0476 expected E2−absE4, got%d", s.IndicatedSpeed)
	}
}

func TestSourceLowAltitudePitchRetainsProductSign(t *testing.T) {
	p, q := sourceFixture(t)
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 18}, RadarAltitude: 16, Rotor: 1, Collective: 0}
	// 16*STAT4c(3)*−16=−768; ABS>>5=24; old negative CWD restores−24;
	// IDIV10=−2; unsigned DF6 produces pitch−12. It must not turn positive.
	if err := Step(&s, p, Input{DF: 6, E0: false, Phase: 1, Y: -16, Switches: 4}, q); err != nil {
		t.Fatal(err)
	}
	if int16(s.Pose.Angles[0]) != -12 {
		t.Fatalf("source pitch sign lost: %d", int16(s.Pose.Angles[0]))
	}
}

func TestContactThresholdsAndFailureOwnership(t *testing.T) {
	p, q := sourceFixture(t)
	base := State{Pose: Pose{H0: 200000, H1: 200000, Height: 3}, RadarAltitude: 1, Vertical: -20, Rotor: 1}
	s := base
	if err := Step(&s, p, Input{DF: 10, Phase: 1}, q); err != nil {
		t.Fatal(err)
	}
	if s.Terminal == nil || s.Terminal.SourceCode != 2 {
		t.Fatalf("source landing threshold should give transient2, got%+v", s.Terminal)
	}
	s = base
	if err := Step(&s, p, Input{DF: 10, Phase: 1, Switches: 0x20}, q); err != nil {
		t.Fatal(err)
	}
	if s.Terminal != nil || s.Pose.Height != 2 || s.Vertical != 0 || s.Forward != 0 {
		t.Fatalf("source no-crash flat contact failed: %+v", s)
	}
	s = base
	q.Height = func(Pose) (int16, error) { return 0, errors.New("synthetic unavailable source query") }
	if err := Step(&s, p, Input{DF: 10, Phase: 1}, q); err == nil || !reflect.DeepEqual(s, base) {
		t.Fatal("failed query partially changed authoritative flight")
	}
}

func TestIndependentLoadAndWindLeaves(t *testing.T) {
	p, q := sourceFixture(t)
	// Source31e4 subtracts mass SAR3 before dividing by STAT46*2 (22*2).
	// With thrust1000 and no ground/speed lift: targets11,0,-22 respectively;
	// the asymmetric source approach moves vertical by at most two.
	for _, tc := range []struct{ mass, want int16 }{{0, 2}, {8000, 0}, {16000, -2}} {
		s := State{Thrust: 1000, RadarAltitude: 200}
		k := kernel{p: p, q: q}
		k.realisticMotion(&s, Input{MassTerm: tc.mass}, 0, 0)
		if s.Vertical != tc.want {
			t.Fatalf("mass%d vertical%d want%d", tc.mass, s.Vertical, tc.want)
		}
	}
	// Source3482 adds ordinary16 plus gust(16+32), not just gust32.
	// A fresh phase1 slot contributes half its value; low product DF6/10
	// yields H1 deltas4 and19. Heading changes by strength*4.
	for _, tc := range []struct {
		enabled, gust bool
		delta         uint32
		yaw           uint16
	}{
		{false, false, 0, 0}, {true, false, 4, 0xffc0}, {true, true, 19, 0xff00},
	} {
		s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 400}, RadarAltitude: 398, Rotor: 1}
		in := Input{DF: 6, Phase: 1, WindStrength: 16, GustActive: tc.gust}
		if tc.enabled {
			in.Switches = 0x80
		}
		if err := Step(&s, p, in, q); err != nil {
			t.Fatal(err)
		}
		if s.Pose.H0 != 200000 || s.Pose.H1 != 200000+tc.delta || s.Pose.Angles[1] != tc.yaw {
			t.Fatalf("wind%+v pose%+v", tc, s.Pose)
		}
	}
}

func TestNoDrawOwnershipAndIdenticalLogicalTraces(t *testing.T) {
	p, q := sourceFixture(t)
	a := State{Pose: Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, Rotor: 1, Collective: 31}
	b := a
	for i := 0; i < 100; i++ {
		in := Input{DF: 6, E0: i%3 != 2, Phase: uint16(i + 1), Switches: 4, Y: -8, Random: func() uint16 { return 1234 }}
		if err := Step(&a, p, in, q); err != nil {
			t.Fatal(err)
		}
		// Presentation reads can occur arbitrarily often; there is no Draw method
		// or clock callback in State or Step to advance the model accidentally.
		for read := 0; read < i%7; read++ {
			_ = b.Pose
		}
		if err := Step(&b, p, in, q); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("logical step%d depends on reads", i)
		}
	}
}

func TestWaypointAngleArgumentOrderHandCardinals(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct {
		h0, h1  uint32
		heading uint16
	}{{200000, 220000, 0}, {220000, 200000, 0xc000}} {
		s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300, Angles: [3]uint16{0, tc.heading, 0}}, RadarAltitude: 298, HeightControl: 1, WaypointIndex: 1}
		s.Waypoints[1] = Waypoint{tc.h0, tc.h1}
		// Source29c0 pushes(deltaH1) then29f4(deltaH0 negated): the first
		// parameter atbp+6 is playerH0−waypointH0. The swapped formula turns
		// a straight north/+H1 route into a quarter turn and is incorrect.
		if err := Step(&s, p, Input{DF: 6, Phase: 1}, q); err != nil {
			t.Fatal(err)
		}
		if s.Pose.Angles[1] != tc.heading {
			t.Fatalf("straight source waypoint changed heading %04x→%04x", tc.heading, s.Pose.Angles[1])
		}
	}
}

func TestHoverTransitionDoesNotConsumeNewDelay(t *testing.T) {
	p, q := sourceFixture(t)
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300}, RadarAltitude: 298, Hover: 2, IndicatedSpeed: 31, Rotor: 1}
	if err := Step(&s, p, Input{DF: 6, E0: true, Phase: 1, Switches: 4}, q); err != nil {
		t.Fatal(err)
	}
	if s.HoverDelay != 16 || s.Hover != 1 {
		t.Fatalf("source new hover delay: %+v", s)
	}
}

func TestTranslationHandCardinalsAndSourceSubtractOrder(t *testing.T) {
	p, q := sourceFixture(t)
	for _, tc := range []struct {
		yaw          uint16
		f, l, dx, dz int16
	}{{0, 120, 0, 0, 60}, {0x4000, 120, 0, -60, 0}, {0xc000, 120, 0, 60, 0}, {0, 0, 10, -5, 0}} {
		s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 300, Angles: [3]uint16{0, tc.yaw, 0}}, RadarAltitude: 298, Forward: tc.f, Lateral: tc.l}
		// 3406/343f SUB AX,SI: the second amplitude minus the saved first.
		// Only the current history slot is initially nonzero; SAR1 gives half.
		if err := Step(&s, p, Input{DF: 10, Phase: 1}, q); err != nil {
			t.Fatal(err)
		}
		if int32(s.Pose.H0)-200000 != int32(tc.dx) || int32(s.Pose.H1)-200000 != int32(tc.dz) {
			t.Fatalf("yaw%04x forward%d lateral%d got(%d,%d), want(%d,%d)", tc.yaw, tc.f, tc.l, int32(s.Pose.H0)-200000, int32(s.Pose.H1)-200000, tc.dx, tc.dz)
		}
	}
}

func TestDistinctHeightHoldClearanceAndRetainedCellConsumer(t *testing.T) {
	p, q := sourceFixture(t)
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 50}, RadarAltitude: 48, Rotor: 1, Hover: 1, LowLimit: 128, HeightTarget: 900}
	if err := Step(&s, p, Input{DF: 6, Phase: 1}, q); err != nil {
		t.Fatal(err)
	}
	// ADf clearance gives(128+50+0+2)SAR1=90; absolute2d04=900
	// is read only by the separate ADB branch and must not be substituted.
	if s.Pose.Height != 90 {
		t.Fatalf("clearance confused with absolute height target: %d", s.Pose.Height)
	}
	s = State{Pose: Pose{H0: 200000, H1: 200000, Height: 50}, RadarAltitude: 48, Rotor: 1, HeightControl: 1, LowLimit: 128, HeightTarget: 400}
	if err := Step(&s, p, Input{DF: 6, Phase: 1}, q); err != nil {
		t.Fatal(err)
	}
	if s.Pose.Height != 400 {
		t.Fatalf("absolute height target ignored: %d", s.Pose.Height)
	}
	calls := 0
	q.Cell = func(Pose) (byte, error) { calls++; return 62, nil }
	q.Height = func(Pose) (int16, error) { return 360, nil }
	s = State{Pose: Pose{H0: 200000, H1: 200000, Height: 380}, RadarAltitude: 18, Rotor: 1, Hover: 1, LowLimit: 128, CellCache: 62}
	if err := Step(&s, p, Input{DF: 6, Phase: 1, Switches: 0x40}, q); err != nil {
		t.Fatal(err)
	}
	if s.Pose.Height != 435 || calls != 0 {
		t.Fatalf("accepted hover path must skip later carrier rejection; height%d calls%d", s.Pose.Height, calls)
	}
	if err := Step(&s, p, Input{DF: 6, Phase: 1, Switches: 0x1040}, q); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || s.CellCache != 62 {
		t.Fatalf("cached source cell refresh gate calls%d cell%d", calls, s.CellCache)
	}
}

func TestAllSixOriginalStatFamiliesHandForces(t *testing.T) {
	_, q := sourceFixture(t)
	for _, tc := range []struct {
		name                     string
		thrust, vertical, height int16
	}{{"APA", 882, 0, 500}, {"COB", 882, 0, 500}, {"DEF", 719, -2, 497}, {"KIA", 719, -2, 497}, {"COM", 882, 0, 500}, {"BLK", 882, -2, 497}} {
		t.Run(tc.name, func(t *testing.T) {
			raw, e := os.ReadFile(testfixture.Path("installation/") + tc.name + "_STAT.DAT")
			if e != nil {
				t.Fatal(e)
			}
			var p Parameters
			copy(p.Raw[:], raw)
			s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 500}, RadarAltitude: 498, Rotor: 1, Collective: 31}
			// Source base mass is STAT42 (7200/7000/6800/8000), distinct
			// from the E0STAT4a pitch limit and the sourceSHHD load percentage.
			mass := int16(binary.LittleEndian.Uint16(raw[0x42:]))
			if e := Step(&s, p, Input{DF: 10, E0: true, Phase: 1, Switches: 4, MassTerm: mass}, q); e != nil {
				t.Fatal(e)
			}
			if s.Thrust != tc.thrust || s.Vertical != tc.vertical || s.Pose.Height != tc.height {
				t.Fatalf("hand table force expected(%d,%d,%d), got(%d,%d,%d)", tc.thrust, tc.vertical, tc.height, s.Thrust, s.Vertical, s.Pose.Height)
			}
		})
	}
}
