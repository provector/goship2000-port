package flightcontrol

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestOriginalKeyboardAxisHandTrace(t *testing.T) {
	a := Axes{}
	// 071d:061e/0626 adds -16 and bounds -127. Eight steps reach
	// -127; release SAR follows -64,-32,-16,-8,-4,-2,-1,0.
	for i := 0; i < 9; i++ {
		a.Advance(Held{Left: true, Up: true})
	}
	if a != (Axes{-127, -127}) {
		t.Fatal(a)
	}
	for _, want := range []int16{-64, -32, -16, -8, -4, -2, -1, 0} {
		a.Advance(Held{})
		if a.X != want || a.Y != want {
			t.Fatalf("release want%d got%+v", want, a)
		}
	}
	a = Axes{-127, 127}
	a.Advance(Held{Left: true, Right: true, Up: true, Down: true})
	if a != (Axes{-111, 127}) {
		t.Fatalf("ordered opposing keys: %+v", a)
	}
	a.Clear()
	if a != (Axes{}) {
		t.Fatal("focus/modal reset retained controls")
	}
}

func TestCollectiveRotorAndHoverSourceBoundaries(t *testing.T) {
	s := State{}
	s.Apply(CollectiveUp)
	if s.Collective != 4 {
		t.Fatal("rotor-off add must be4")
	}
	s.Apply(RotorToggle)
	if s.Rotor != 1 || s.FlightStage2D08 != 4 {
		t.Fatal(s)
	}
	s.Apply(CollectiveUp)
	if s.Collective != 5 {
		t.Fatal("rotor-on normal add must be1")
	}
	s.Apply(CollectiveUpFast)
	if s.Collective != 9 {
		t.Fatal("shift add must be4")
	}
	for i := 0; i < 40; i++ {
		s.Apply(CollectiveUpFast)
	}
	if s.Collective != 31 {
		t.Fatal(s)
	}
	for _, tc := range []struct{ speed, want int16 }{{63, 1}, {64, 2}, {-63, 1}, {-64, 2}, {-32768, 1}} {
		s.Hover, s.SpeedE2, s.SpeedHold2D06 = 0, tc.speed, 100
		s.Apply(HoverToggle)
		if s.Hover != tc.want || s.SpeedHold2D06 != 0 {
			t.Fatalf("source wordABS/strict64 speed%d: %+v", tc.speed, s)
		}
	}
	s.Hover, s.AvionicsDamage = 0, 2
	if s.Apply(HoverToggle) {
		t.Fatal("source damage guard changed control state")
	}
	s.Apply(RotorToggle)
	if s.Rotor != 0 || s.Collective != 0 {
		t.Fatal("disengagement must bottom collective")
	}
}

func TestControlSelectionAndLimitSourceRules(t *testing.T) {
	s := State{LowLimit: 64}
	s.Apply(LowLimitDown)
	if s.LowLimit != 64 {
		t.Fatal("native lowlimit minimum64")
	}
	for i := 0; i < 7; i++ {
		s.Apply(LowLimitUp)
	}
	if s.LowLimit != 320 {
		t.Fatal("native lowlimit maximum320")
	}
	s.PrimaryMFD, s.SecondaryMFD = 5, 5
	s.Apply(PrimaryMFDNext)
	s.Apply(SecondaryMFDNext)
	if s.PrimaryMFD != 0 || s.SecondaryMFD != 5 {
		t.Fatal("absent secondMFD guard")
	}
	s.SecondaryMFDInstalled = true
	s.Apply(SecondaryMFDNext)
	if s.SecondaryMFD != -1 {
		t.Fatal("secondMFD has distinct attitude−1")
	}
	if Resolve("p", false, true) != Pause || Resolve("=", true, false) != CollectiveUpFast || Resolve("9", false, false) != RotorToggle {
		t.Fatal("recovered keyboard bindings")
	}
	for _, key := range []string{"Enter", "Escape", "F10", "Space", "3", "8", "5", "6", "7", "[", "]"} {
		if Resolve(key, false, false) != None {
			t.Fatalf("unimplemented or context-dependent key%s intercepted", key)
		}
	}
	if Resolve("9", true, false) != None {
		t.Fatal("source Shift9 is a different BIOS key")
	}
}

func TestControlSourceInstructionWindows(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file int
		hex  string
	}{
		{0x77d0 + 0x60e, "d1fb3b1e3413752233db"},
		{0x77d0 + 0x61e, "83eb1083fb817d03bb81ff"},
		{0x77d0 + 0x62d, "83c31083fb7f7e03bb7f00"},
		{0xd8b0 + 0x7c2, "26833ee10a011bc025030040"},
		{0xd8b0 + 0x822, "8e06e832268306af0a04"},
		{0xd8b0 + 0xc48, "a1e2009933c22bc23d4000"},
		{0xd8b0 + 0xe86, "8e060033268336e10a01"},
		{0xd8b0 + 0xea0, "8e06e83226c706af0a0000"},
		{0xd8b0 + 0x10c4, "8e06163326833edf0a407f03"},
		{0xd8b0 + 0x11be, "8e06163326813edf0a40017c03"},
		{0xd8b0 + 0x11fe, "26c706d90affff"},
		{0xd8b0 + 0x121c, "26c706d70a0000"},
	} {
		want, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw[tc.file:tc.file+len(want)], want) {
			t.Fatalf("original instruction mismatch file%x", tc.file)
		}
	}
}
