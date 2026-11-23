package cockpit

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestHUDWaypointMarkerOriginalOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional immutable source operand check")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		ip int
		b  []byte
	}{
		{0x26d4, []byte{0x26, 0x80, 0x3e, 0x88, 0x04, 0x00}},
		{0x2767, []byte{0x2b, 0x46, 0xfc, 0x8a, 0xc4, 0x98, 0x05, 0xa0, 0x00}},
		{0x2789, []byte{0x6a, 0x01, 0x6a, 0x04, 0xff, 0x76, 0xf0, 0x0e, 0x68, 0x76, 0x34}},
	} {
		if !bytes.Equal(b[0x1dc60+v.ip:0x1dc60+v.ip+len(v.b)], v.b) {
			t.Fatalf("original section2:%04x", v.ip)
		}
	}
	// The green down-pointing HUD triangle is prepared original art, not the
	// unrelated resident GPS numbered triangles or gray infantry vector.
	want := []byte{7, 0, 4, 0, 26, 26, 26, 26, 26, 26, 26, 0, 26, 26, 26, 26, 26, 0, 0, 0, 26, 26, 26, 0, 0, 0, 0, 0, 26, 0, 0, 0}
	if !bytes.Equal(b[0x1dc60+0x3476:0x1dc60+0x3476+len(want)], want) {
		t.Fatal("original7x4 HUD triangle artwork changed")
	}
}

func TestHUDWaypointMarkerBearingAndVisibility(t *testing.T) {
	a := testAssets()
	a.Sprites["waypoint-marker"] = Image{Width: 7, Height: 4, Pixels: []byte{0, 0, 0, 10, 0, 0, 0, 0, 0, 10, 10, 10, 0, 0, 0, 10, 10, 10, 10, 10, 0, 10, 10, 10, 10, 10, 10, 10}}
	for _, v := range []struct {
		bearing, heading uint16
		x                int
	}{
		{0, 0, 160}, {0x1000, 0, 176}, {0, 0x1000, 144}, {0x7f00, 0, 224}, {0x8000, 0, 96}, {0xffff, 0, 159},
	} {
		p := bytes.Repeat([]byte{77}, 64000)
		nav := &NavigationBearing{Active: true, Bearing: v.bearing, HUDMarker: true}
		if err := a.drawHUDNavigationMarker(p, v.heading, nav); err != nil {
			t.Fatal(err)
		}
		if p[4*320+v.x] != 10 || p[7*320+v.x-3] != 10 || p[4*320+v.x-3] != 77 {
			t.Fatal(v, "anchor/transparent00/sourcecolour")
		}
		before := append([]byte(nil), p...)
		nav.Active = false
		if err := a.drawHUDNavigationMarker(p, v.heading, nav); err != nil || !bytes.Equal(before, p) {
			t.Fatal("inactive waypoint drew", err)
		}
	}
	// Historical optional carrier does not change canonical JSON or require art.
	old, err := json.Marshal(&NavigationBearing{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(old, []byte("HUDMarker")) {
		t.Fatal("historical zero field serialized")
	}
	delete(a.Sprites, "waypoint-marker")
	p := make([]byte, 64000)
	if err := a.drawHUDNavigationMarker(p, 0, &NavigationBearing{Active: true}); err != nil {
		t.Fatal("historical rendering newly requires sprite", err)
	}
}

func TestHUDWaypointMarkerComposeOrderAndDamage(t *testing.T) {
	a := hudTestAssets()
	a.Sprites["waypoint-marker"] = Image{Width: 7, Height: 4, Pixels: bytes.Repeat([]byte{10}, 28)}
	for _, mode := range []int{0, 1, 3, 7} {
		for _, damage := range []byte{0, 0x80} {
			in := Inputs{AircraftID: 2, HUD: true, CameraMode: mode, Navigation: &NavigationBearing{Active: true, Bearing: 0, HUDMarker: true}}
			in.Damage[0] = damage
			p := bytes.Repeat([]byte{77}, 64000)
			if err := Compose(p, a, in); err != nil {
				t.Fatal(err)
			}
			if (p[6*320+160] == 10) != (damage == 0) {
				t.Fatalf("view%d damage%02x common HUD gate", mode, damage)
			}
		}
	}
}
