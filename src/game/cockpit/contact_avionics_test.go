package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestContactThreatPixelsGatesAndDrawPurity(t *testing.T) {
	for id := 0; id < 8; id++ {
		family, _ := Family(id)
		d := Definition{}
		for off, value := range map[int]int16{0x5c: 100, 0x5e: 100, 0x60: 80, 0x62: 80, 0x64: 120, 0x66: 120} {
			d.Parameters[off/2] = value
		}
		a := &Assets{Definitions: map[string]Definition{family: d}, Sprites: map[string]Image{"target-marker": {Width: 5, Height: 3, Pixels: bytes.Repeat([]byte{31}, 15)}}}
		in := Inputs{AircraftID: id, HUD: true, SecondaryMFD: 5, BlinkPhase: 2, ContactAvionics: &ContactAvionicsInputs{ThreatPoints: []ContactPoint{{X: 3, Y: -2, Colour: 7}, {X: 5, Y: 1, Colour: 12}, {X: -3, Y: -2, Colour: 12, Attack: true}, {X: -20, Y: 0, Colour: 6}}}}
		before := *in.ContactAvionics
		dst := make([]byte, 64000)
		// Hand boundary from CMP AX,0050h:79 admits;80 rejects. Axes and
		// accepted attack neighbors call1f20 without another destination test.
		dst[98*320+103] = 79
		dst[101*320+105] = 80
		dst[98*320+97] = 79
		dst[98*320+96] = 80
		dst[100*320+100] = 0x90
		dst[100*320+80] = 0x90
		logical := append([]byte(nil), dst...)
		if err := a.DrawThreatBackground(logical, id, 0); err != nil {
			t.Fatal(err)
		}
		if logical[100*320+100] != 10 || logical[100*320+80] != 2 || logical[98*320+103] != 79 {
			t.Fatal("source raw markers must precede logical contact readback")
		}
		if err := a.DrawContactAvionics(dst, in); err != nil {
			t.Fatal(err)
		}
		if dst[98*320+103] != 7 || dst[101*320+105] != 80 || dst[100*320+100] != 10 || dst[100*320+80] != 6 || dst[98*320+96] != 13 {
			t.Fatalf("source pixel/mask carrieraircraft%d", id)
		}
		if !reflect.DeepEqual(before, *in.ContactAvionics) {
			t.Fatal("drawing advanced snapshot")
		}
		in.Damage[1] = 0x40
		again := make([]byte, 64000)
		if err := a.DrawContactAvionics(again, in); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, make([]byte, 64000)) {
			t.Fatal("damage gate ignored")
		}
	}
}

func TestOriginalThreatPixelAcceptanceAndRawWrites(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original instruction witnesses")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Section2 original1975 uses file1dc60+IP. Center acceptance is80;
	// marker and cross sites call the pixel writer directly, not1974 readback.
	for ip, h := range map[int]string{0x3870: "3d5000", 0x35e9: "9a201f7f03", 0x360d: "9a201f7f03", 0x3958: "9a201f7f03", 0x396a: "9a201f7f03", 0x397c: "9a201f7f03", 0x398e: "9a201f7f03"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0x1dc60+ip:0x1dc60+ip+len(want)], want) {
			t.Fatalf("original1975:%04x differs", ip)
		}
	}
}
