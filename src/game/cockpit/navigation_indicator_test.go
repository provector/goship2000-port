package cockpit

import (
	"bytes"
	"testing"
)

func TestINSBearingSourceHandCases(t *testing.T) {
	a := hudTestAssets()
	i := a.Sprites["bearing-marker"]
	i.Pixels[0] = 31
	a.Sprites["bearing-marker"] = i
	d := Definition{}
	for off, value := range map[int]int16{0x3c: 100, 0x3e: 90, 0x40: 120, 0x42: 99, 0x44: 110, 0x46: 94, 0x48: 101, 0x4a: 101} {
		d.Parameters[off/2] = value
	}
	for _, q := range []struct {
		bearing, heading uint16
		x                int
		digits           [3]byte
	}{
		{0, 0, 107, [3]byte{1, 1, 1}},
		{0x4000, 0, 115, [3]byte{1, 9, 10}}, //16384/183=89; high40SAR3=8.
		{0xffff, 0, 106, [3]byte{4, 6, 9}},  //65535/183=358; signedFFSAR3=-1.
		{0, 0x0100, 106, [3]byte{1, 1, 1}},
		{0x8000, 0, 97, [3]byte{2, 8, 10}}, //signed80SAR3=-16,clampedX100; sprite-3.
	} {
		p := bytes.Repeat([]byte{77}, 64000)
		if err := a.drawNavigationIndicator(p, d, q.heading, &NavigationBearing{Active: true, Bearing: q.bearing}); err != nil {
			t.Fatal(err)
		}
		for j, v := range q.digits {
			if p[101*320+101+7*j] != v {
				t.Fatalf("%+v digit%d=%d", q, j, p[101*320+101+7*j])
			}
		}
		// At left boundary the first spritepixel is clipped rather than moved.
		if q.x >= 100 && p[94*320+q.x] != 31 {
			t.Fatalf("%+v marker at%d", q, q.x)
		}
		if q.x < 100 && p[94*320+q.x] == 31 {
			t.Fatal("marker escaped suppliedCPITclip")
		}
	}
	p := bytes.Repeat([]byte{77}, 64000)
	if err := a.drawNavigationIndicator(p, d, 0xffff, nil); err != nil {
		t.Fatal(err)
	}
	if p[94*320+107] != 31 || p[101*320+101] != 1 || p[101*320+108] != 1 || p[101*320+115] != 1 {
		t.Fatal("selector0 mustdrawcenteredmarker+000")
	}
}

func TestINSComposeGatesAndRefresh(t *testing.T) {
	a := hudTestAssets()
	d := a.Definitions["COB"]
	for off, value := range map[int]int16{0x3c: 222, 0x3e: 115, 0x40: 241, 0x42: 121, 0x44: 230, 0x46: 118, 0x48: 221, 0x4a: 123} {
		d.Parameters[off/2] = value
	}
	a.Definitions["COB"] = d
	for _, q := range []struct {
		camera, arr int
		damage      byte
		visible     bool
	}{{0, 0, 0, true}, {0, 1, 0, true}, {0, 2, 0, false}, {0, 3, 0, true}, {3, 0, 0, false}, {7, 0, 0, false}, {0, 0, 0x10, false}} {
		base := Inputs{AircraftID: 2, HUD: true, CameraMode: q.camera, Arrangement: q.arr}
		base.Damage[0] = q.damage
		p := make([]byte, 64000)
		if err := Compose(p, a, base); err != nil {
			t.Fatal(err)
		}
		got := p[123*320+221] != 0
		if got != q.visible {
			t.Fatalf("%+v visible%v", q, got)
		}
	}
}
