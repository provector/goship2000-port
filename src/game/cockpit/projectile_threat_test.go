package cockpit

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func threatAssets(id, cx, cy int) *Assets {
	family, _ := Family(id)
	d := Definition{}
	for off, v := range map[int]int{0x5c: cx, 0x5e: cy, 0x60: cx - 20, 0x62: cy - 16, 0x64: cx + 20, 0x66: cy + 16} {
		d.Parameters[off/2] = int16(v)
	}
	return &Assets{Definitions: map[string]Definition{family: d}}
}
func TestThreatExactOriginalEightyOnePixels(t *testing.T) {
	before, e := os.ReadFile(testfixture.Path("threat-display/threat01-before.gray8"))
	if os.IsNotExist(e) {
		t.Skip("optional captured witness")
	}
	if e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(testfixture.Path("threat-display/threat01-after.gray8"))
	if e != nil {
		t.Fatal(e)
	}
	if len(before) != 81 || len(after) != 81 {
		t.Fatal("capture framing")
	}
	a := threatAssets(2, 244, 184)
	dst := make([]byte, 64000)
	for y := 0; y < 9; y++ {
		copy(dst[(180+y)*320+240:], before[y*9:(y+1)*9])
	}
	if !a.DrawThreatProjectilePoint(dst, 2, ProjectilePoint{}) {
		t.Fatal("captured operation rejected")
	}
	got := make([]byte, 81)
	for y := 0; y < 9; y++ {
		copy(got[y*9:(y+1)*9], dst[(180+y)*320+240:(180+y)*320+249])
	}
	if !bytes.Equal(got, after) {
		t.Fatalf("captured M247 rectangle differs\ngot%x\nwant%x", got, after)
	}
	changes := 0
	for i := range before {
		if before[i] != after[i] {
			changes++
			if i != 4*9+4 || before[i] != 10 || after[i] != 14 {
				t.Fatal("unexpected original witness change")
			}
		}
	}
	if changes != 1 {
		t.Fatal(changes)
	}
}
func TestProjectilePaletteGuardRawCrossAndAllCockpitGates(t *testing.T) {
	for id := 0; id < 8; id++ {
		a := threatAssets(id, 100, 100)
		dst := make([]byte, 64000)
		// No CPIT clipping after rotation: x+21 remains admissible on index79.
		dst[100*320+121] = 79
		if !a.DrawThreatProjectilePoint(dst, id, ProjectilePoint{X: 21}) || dst[100*320+121] != 14 {
			t.Fatal(id, "extra rectangle clipping")
		}
		dst[100*320+100] = 80
		if a.DrawThreatProjectilePoint(dst, id, ProjectilePoint{}) {
			t.Fatal("index80 admitted")
		}
		dst[100*320+100] = 79
		dst[100*320+99] = 100
		if !a.DrawThreatProjectilePoint(dst, id, ProjectilePoint{Cross: true}) || dst[100*320+99] != 14 {
			t.Fatal("raw cross neighbor")
		}
		in := Inputs{AircraftID: id, HUD: true, SecondaryMFD: 5, ContactAvionics: &ContactAvionicsInputs{ProjectilePoints: []ProjectilePoint{{X: 3, Y: 2}}}}
		snapshot := *in.ContactAvionics
		for _, arr := range []int{0, 1, 2, 3} {
			in.Arrangement = arr
			dst = make([]byte, 64000)
			if e := a.DrawContactAvionics(dst, in); e != nil {
				t.Fatal(e)
			}
			if (dst[102*320+103] == 14) != (arr != 2) {
				t.Fatal("arrangement gate", id, arr)
			}
		}
		for _, mode := range []int{1, 3, 5, 7} {
			in.Arrangement = 0
			in.CameraMode = mode
			dst = make([]byte, 64000)
			if e := a.DrawContactAvionics(dst, in); e != nil {
				t.Fatal(e)
			}
			if dst[102*320+103] != 0 {
				t.Fatal("camera gate")
			}
		}
		if !reflect.DeepEqual(snapshot, *in.ContactAvionics) {
			t.Fatal("drawing mutated projectile snapshot")
		}
	}
}

func TestProjectileScaledPhysicalDestinationAndOrderedContacts(t *testing.T) {
	a := threatAssets(2, 100, 100)
	dst := make([]byte, 256000)
	// Four physical subpixels retain distinct destination indices. The source
	// logical guard is performed against each plane, never a downsampled average.
	dst[200*640+200] = 79
	dst[200*640+201] = 80
	dst[201*640+200] = 10
	dst[201*640+201] = 200
	q := ProjectilePoint{}
	if e := LogicalPass(dst, 2, func(logical []byte) error { a.DrawThreatProjectilePoint(logical, 2, q); return nil }); e != nil {
		t.Fatal(e)
	}
	if dst[200*640+200] != 14 || dst[200*640+201] != 80 || dst[201*640+200] != 14 || dst[201*640+201] != 200 {
		t.Fatal("physical palette guard lost")
	}
	logical := make([]byte, 64000)
	if !a.DrawThreatContactPoint(logical, 2, 0, ContactPoint{Colour: 0x60}) {
		t.Fatal("contact rejected")
	}
	if a.DrawThreatProjectilePoint(logical, 2, q) || logical[100*320+100] != 0x60 {
		t.Fatal("projectile ignored preceding contact write")
	}
}
