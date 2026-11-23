package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestWarningOriginalOperandsAndPaletteMasks(t *testing.T) {
	source, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, want := range map[int]string{0x3c34: "f646f804", 0x3c60: "f646f840", 0x3c66: "f646f920", 0x3c6c: "f606da0001", 0x3c99: "f646f880", 0x3cc5: "f606de0040", 0x3ccc: "f606da0003", 0x3cd3: "6a069a0b004408"} {
		b, _ := hex.DecodeString(want)
		if !bytes.Equal(source[0x1dc60+ip:][:len(b)], b) {
			t.Fatal(ip)
		}
	}
	for id := 0; id < 8; id++ {
		f, _ := Family(id)
		d := Definition{Colours: [6]byte{3, 10, 8, 15, 65, 68}}
		for off, xy := range map[int][2]int16{0x6c: {10, 20}, 0x70: {30, 20}, 0x74: {50, 20}} {
			d.Parameters[off/2], d.Parameters[(off+2)/2] = xy[0], xy[1]
		}
		a := &Assets{Definitions: map[string]Definition{f: d}}
		for _, flags := range []uint16{0, 4, 0x40, 0x80, 0x2000, 0x20, 0x2004, 0xc4} {
			for _, phase := range []uint16{0, 1, 2, 3, 4} {
				p := bytes.Repeat([]byte{65}, 64000)
				p[20*320+11] = 99
				in := Inputs{AircraftID: id, HUD: true, SecondaryMFD: 5, BlinkPhase: phase, ContactAvionics: &ContactAvionicsInputs{WarningFlags: flags}}
				if e = a.DrawContactAvionics(p, in); e != nil {
					t.Fatal(e)
				}
				for _, row := range []struct {
					x  int
					on bool
				}{{10, flags&4 != 0}, {30, flags&0x80 != 0}, {50, flags&0x40 != 0 || flags&0x2000 != 0 && phase&1 != 0}} {
					want := byte(65)
					if row.on {
						want = 68
					}
					if p[20*320+row.x] != want || p[25*320+row.x+9] != want || p[26*320+row.x] != 65 || p[20*320+row.x+10] != 65 {
						t.Fatal(id, flags, phase, row)
					}
				}
				if p[20*320+11] != 99 {
					t.Fatal("nonmatching ink corrupted")
				}
			}
		}
	}
}

func TestWarningViewAndDamageGatesClearWithFreshFrame(t *testing.T) {
	f, _ := Family(0)
	d := Definition{Colours: [6]byte{3, 10, 8, 15, 65, 68}}
	d.Parameters[0x74/2], d.Parameters[0x76/2] = 30, 20
	a := &Assets{Definitions: map[string]Definition{f: d}}
	base := Inputs{AircraftID: 0, HUD: true, SecondaryMFD: 5, ContactAvionics: &ContactAvionicsInputs{WarningFlags: 0x40}}
	for _, what := range []string{"healthy", "HUDoff", "external", "mast", "arrangement2", "differentMFD", "sensorDamage", "defenseDamage"} {
		in := base
		switch what {
		case "HUDoff":
			in.HUD = false
		case "external":
			in.CameraMode = 5
		case "mast":
			in.CameraMode = 7
		case "arrangement2":
			in.Arrangement = 2
		case "differentMFD":
			in.SecondaryMFD = 0
		case "sensorDamage":
			in.Damage[1] = 0x40
		case "defenseDamage":
			in.Damage[0] = 0x40
		}
		p := bytes.Repeat([]byte{65}, 64000)
		if e := a.DrawContactAvionics(p, in); e != nil {
			t.Fatal(e)
		}
		want := byte(65)
		if what == "healthy" || what == "defenseDamage" {
			want = 68
		}
		if p[20*320+30] != want {
			t.Fatal(what)
		}
	}
}
