package flight

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestAltitudeWarningIndependentOperands(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, h := range map[int]string{0x38bf: "803ede0000", 0x38c6: "f606de0040", 0x38f2: "6a04", 0x38f6: "813ea03bdc05", 0x3905: "6a05", 0x3907: "9a0b004408"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("warning source%04x differs", ip)
		}
	}
	for _, tc := range []struct {
		height, terrain, low, vertical int16
		gate, want                     byte
	}{
		{1499, 0, 128, 1, 0x40, 0}, {1500, 0, 128, 1, 0x40, 0}, {1501, 0, 128, 1, 0x40, 5},
		{1921, 0, 128, 1, 0x40, 5}, {1501, 0, 128, 0, 0x40, 0}, {1501, 0, 128, -1, 0x40, 0},
		{1501, 0, 128, 1, 0, 0}, {1501, 0, 128, 1, 0x20, 0}, {1501, 0, 128, 1, 0x80, 0}, {1501, 0, 128, 1, 0xff, 5},
		{102, 100, 128, -1, 0x40, 0}, {103, 100, 128, -1, 0x40, 4}, {229, 100, 128, -1, 0x40, 4}, {230, 100, 128, -1, 0x40, 0},
		{103, 100, 128, 0, 0x40, 0}, {103, 100, 128, 1, 0x40, 0},
	} {
		if got := AltitudeWarning(tc.height, tc.terrain, tc.low, tc.vertical, tc.gate); got != tc.want {
			t.Fatalf("warning%+v got%d", tc, got)
		}
	}
}

func TestAltitudeWarningAcceptedPhaseBeforeCeiling(t *testing.T) {
	p, q := sourceFixture(t)
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 1921}, Vertical: 1, Rotor: 1, Collective: 16, RadarAltitude: 1919}
	var events []byte
	if e := Step(&s, p, Input{DF: 6, SourceSoundGate: 0x40, Warning: func(e byte) { events = append(events, e) }}, q); e != nil {
		t.Fatal(e)
	}
	if len(events) != 1 || events[0] != 5 || s.Pose.Height != 1920 {
		t.Fatalf("events%v height%d", events, s.Pose.Height)
	}
}
