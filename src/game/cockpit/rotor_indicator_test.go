package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestRotorIndicatorSourceCarriersAndGuards(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original source unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	for ip, literal := range map[int]string{0x652: "26833ee10a00", 0x66b: "684c34", 0x674: "26833eaf0a00", 0x67c: "f606da0004", 0x693: "8e06ce3326ff36b21926ff36b019"} {
		want, e := hex.DecodeString(literal)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(raw[0x1dc60+ip:][:len(want)], want) {
			t.Fatalf("source section2 original1975:%04x changed", ip)
		}
	}
}

func TestRotorIndicatorHandPixelsAndLogicalPhase(t *testing.T) {
	a := &Assets{Sprites: map[string]Image{
		"warning":       {Width: 4, Height: 4, Pixels: []byte{12, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 15}},
		"driver-marker": {Width: 4, Height: 4, Pixels: []byte{13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 14}},
	}}
	d := Definition{}
	d.Parameters[0x88/2] = 10
	d.Parameters[0x8a/2] = 20
	for _, tc := range []struct {
		rotor, collective int16
		phase             uint16
		damage            byte
		want              byte
	}{{0, 0, 4, 0, 0x55}, {0, 1, 0, 0, 0x55}, {0, 1, 4, 0, 13}, {1, 0, 0, 0, 12}, {1, 0, 4, 0, 12}, {1, 16, 4, 0x10, 0x55}} {
		in := Inputs{Rotor: tc.rotor, Bars: &BarInputs{LoadOrdinal: tc.collective}, BlinkPhase: tc.phase}
		in.Damage[0] = tc.damage
		for repeat := 0; repeat < 3; repeat++ {
			frame := bytes.Repeat([]byte{0x55}, 64000)
			if e := a.drawRotorIndicator(frame, d, in); e != nil {
				t.Fatal(e)
			}
			if frame[20*320+10] != tc.want || frame[20*320+11] != 0x55 {
				t.Fatalf("source flags4 anchor/zero mask %+v got%x", tc, frame[20*320+10:20*320+14])
			}
		}
	}
	if a.Sprites["warning"].Pixels[0] != 12 {
		t.Fatal("shared rotor carrier mutated")
	}
}
