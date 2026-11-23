package combat

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func deathFixture(t *testing.T, kind byte) (*State, Input) {
	s, in := fixture(t, 0)
	s.assets.Models = []string{"flat", "flat", "flat"}
	e := &s.init.Entities[1]
	e.Kind, e.Participation, e.RawMotion = kind, 2, 4
	e.ModelID, e.ReplacementModelID = 1, 2
	e.Pose.Height, e.Visible = 0, true
	e.Object.Raw[12] = 1
	return s, in
}

func TestDeathSourceDispatchAndDistinctCounterBranches(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
	// The table operand is relative toCS10bb, while these window labels are
	// original0975; treating880e as an0975 offset selects projectile code.
	for ip, literal := range map[int]string{
		0xfc69: "2effa70e88", 0xc07a: "268b87882526ff8f88253d0000",
		0xc817: "268b87882526ff8f88253d0000", 0xd59d: "26ff8f88252683bf882500",
		0xda07: "26ff8f88252683bf882500", 0xdbe8: "26ff8f88252683bf882500",
		0xd5f0: "26c68782251c26c7878825040026c687832500",
		0x9e63: "26c7878a2500fe", 0x9f33: "2681af8a250002",
		0x9ef3: "b91300f3a5", 0xdd92: "268a87420624f90c01",
	} {
		want, _ := hex.DecodeString(literal)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("section1 source%04x changed", ip)
		}
	}
	for kind, alias := range map[int]uint16{0: 0x3be8, 1: 0x4280, 3: 0x465e, 8: 0x5c57, 0xa: 0x4ca6, 0x15: 0x5bcb, 0x17: 0x5c57, 0x19: 0x61d1, 0x1b: 0x663b, 0x1c: 0x681c, 0x22: 0x7434, 0x26: 0x7fbf} {
		if got := binary.LittleEndian.Uint16(b[0x14d10+0x880e+kind*2:]); got != alias {
			t.Fatalf("CS10bb tablekind%02x=%04x want%04x", kind, got, alias)
		}
	}
}

func TestStationaryDestructionCountdownThenWreckObservation(t *testing.T) {
	for _, kind := range []byte{2, 3, 7, 0xa, 8, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1b} {
		s, in := deathFixture(t, kind)
		e := &s.init.Entities[1]
		steps := 5 // P2→3; fourDEC and post-DEC zero test.
		if kind >= 2 && kind <= 7 || kind == 0xa {
			steps = 6 // C07A/C817 requires pre-DEC valuezero.
		}
		for tick := 1; tick <= steps; tick++ {
			rows, err := s.AdvanceDeaths(in)
			if err != nil || len(rows) != 0 {
				t.Fatalf("kind%02x tick%d rows%v err%v", kind, tick, rows, err)
			}
			if tick < steps && (e.Kind != kind || e.ModelID != 1) {
				t.Fatalf("kind%02x replacedearly tick%d %+v", kind, tick, e)
			}
		}
		if e.Kind != 0x1c || e.Participation != 0 || e.ModelID != 2 || !e.Visible || e.Object.Raw[12] != 1 {
			t.Fatalf("kind%02x badreplacement %+v", kind, e)
		}
		if len(events(s, "wreckmodel-ready")) != 1 {
			t.Fatal("replacement event must be exactly once")
		}
		rows, err := s.AdvanceDeaths(in)
		if err != nil || len(rows) != 1 || rows[0] != 1 || e.Participation != 4 || e.Pose.Height != 0 {
			t.Fatalf("kind%02x next-updatecontact rows%v err%v", kind, rows, err)
		}
		for tick := 0; tick < 20; tick++ {
			rows, err = s.AdvanceDeaths(in)
			if err != nil || len(rows) != 0 || len(events(s, "wreckmodel-ready")) != 1 {
				t.Fatal("settled wreck reinitialized or repeatedreplacement")
			}
		}
	}
}

func TestStationaryCounterUsesEqualityAndSourceWordWrap(t *testing.T) {
	for _, kind := range []byte{3, 0x17} {
		s, in := deathFixture(t, kind)
		e := &s.init.Entities[1]
		e.Participation, e.RawMotion = 3, -32768
		if _, err := s.AdvanceDeaths(in); err != nil {
			t.Fatal(err)
		}
		if e.RawMotion != 32767 || e.Kind != kind || e.Participation != 3 {
			t.Fatal("word wrap was replaced by nonpositive clamp")
		}
	}
}

func TestStationaryGroupLeaderAndPlayerSafety(t *testing.T) {
	s, in := deathFixture(t, 0x17)
	e := &s.init.Entities[1]
	e.Participation, e.RawMotion, e.DynamicRaw[2], e.LeaderRow = 3, 1, 0x21, 2
	if _, err := s.AdvanceDeaths(in); err != nil {
		t.Fatal(err)
	}
	if s.init.Entities[2].Participation != 4 || e.DynamicRaw[2] != 0 {
		t.Fatal("source route leader not released")
	}
	p := &s.init.Entities[0]
	p.Kind, p.Participation = 0x21, 2
	before := *p
	s.AdvanceDeaths(in)
	if *p != before {
		t.Fatal("stationary NPC destruction owner changed player state")
	}
}

func TestFallFamilyHistoryAndTerminalInterfaces(t *testing.T) {
	for _, kind := range []byte{0, 1, 0x10, 0x11, 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26} {
		s, in := deathFixture(t, kind)
		e := &s.init.Entities[1]
		e.Pose.Height = 2000
		e.Pose.Angles[0] = 0
		e.ControlRaw[1], e.ControlRaw[3] = 3, 3
		startH0, startH1 := e.Pose.H0, e.Pose.H1
		in.Random = func() uint16 { t.Fatal("death mechanism consumed RNG"); return 0 }
		if _, err := s.AdvanceDeaths(in); err != nil {
			t.Fatal(err)
		}
		if e.Kind != kind || e.Pose.Angles[0] != 0xfe00 {
			t.Fatal("first source pitch assignment/decrement")
		}
		// DF2: ((2<<1)<<8)/10 =102. The original sine table has
		// quarter[32]=804, so(-804*102)SAR14=-6; yaw0 adds102 toH1.
		if e.Pose.H0 != startH0 || e.Pose.H1 != startH1+102 || e.Pose.Height != 1994 {
			t.Fatalf("hand-derived first fall pose: %+v", e.Pose)
		}
		if _, err := s.AdvanceDeaths(in); err != nil {
			t.Fatal(err)
		}
		wantPitch := uint16(0xfc00)
		if kind <= 1 || kind == 0x10 || kind == 0x11 {
			wantPitch = 0xfe00
		}
		if e.Pose.Angles[0] != wantPitch {
			t.Fatal("fall families collapsed", kind, e.Pose.Angles)
		}
		wantHeight := uint16(1984) // quarter[64]=1606: (-1606*102)SAR14=-10.
		if wantPitch == 0xfe00 {
			wantHeight = 1988
		}
		if e.Pose.H1 != startH1+204 || e.Pose.Height != wantHeight {
			t.Fatal("second hand-derived fall pose", e.Pose)
		}
		// Atground, this same mechanism settles without an AI decision.
		e.Pose.Height = 0
		if _, err := s.AdvanceDeaths(in); err != nil {
			t.Fatal(err)
		}
		if e.Kind != 0x1c || e.Participation != 0 || e.ModelID != 2 || int16(binary.LittleEndian.Uint16(e.DynamicRaw[4:])) != -100 {
			t.Fatal("fall did not reach replacement", kind, e)
		}
		advanced := kind >= 0x20
		if advanced && (e.ControlRaw[1] != 0 || e.ControlRaw[3] != 0 || s.Results[3].Kind != 3 || len(events(s, "contact-stale")) != 1 || events(s, "contact-stale")[0].Target != 2) {
			t.Fatal("advancedfall missing source control/result/contact cleanup")
		}
		if !advanced && (s.Results[3] != (Result{}) || e.ControlRaw[3] != 3) {
			t.Fatal("classicfall inherited advanced cleanup")
		}
	}
}

func TestReplacementFailurePreservesUnknownModelDomain(t *testing.T) {
	s, in := deathFixture(t, 3)
	e := &s.init.Entities[1]
	e.Participation, e.RawMotion, e.ReplacementModelID = 3, 0, 9
	if _, err := s.AdvanceDeaths(in); err == nil {
		t.Fatal("unknown replacement silently substituted")
	}
	if e.Kind != 3 || e.ModelID != 1 {
		t.Fatal("failed replacement changed model identity")
	}
}

func TestWreckVisualSwapDoesNotRegisterReplacementHeightRegions(t *testing.T) {
	s, in := deathFixture(t, 3)
	e := &s.init.Entities[1]
	e.Participation, e.RawMotion = 3, 0
	if _, err := s.AdvanceDeaths(in); err != nil {
		t.Fatal(err)
	}
	objects := s.deathQueryObjects()
	if objects[1].ModelID != 1 || e.ModelID != 2 {
		t.Fatal("visual header replacement changed original registered query prefix")
	}
	if _, err := s.AdvanceDeaths(in); err != nil {
		t.Fatal(err)
	}
	if len(s.deathQueryObjects()) != len(objects)-1 {
		t.Fatal("DD3E wreck initialization did not disable its query prefix")
	}
}
