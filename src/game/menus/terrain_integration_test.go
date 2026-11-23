package menus

import (
	"gunship2000-assets/game/render3d"
	"testing"
)

func TestActualMissionTerrainReplacesOldTarmac(t *testing.T) {
	c := mastFixture(t, 2, "land")
	p := c.Rendering
	if p.Scene.World != "GULF5" {
		t.Fatalf("fixtureworld%s", p.Scene.World)
	}
	s := p.Scene
	s.Environment = nil
	// Genuine generated world/population, supplied adjacent camera poses from
	// the retained K reproduction (not a reconstructed user screenshot).
	dst := make([]byte, 64000)
	for _, i := range []int{37, 38} {
		s.Camera = render3d.Camera{Pose: render3d.Pose{H0: 354856 + uint32(i*12), H1: 349664 + uint32(i*16), Height: 120}, Angles: [3]uint16{0xf000, 0x4000 + uint16(i*64), uint16(-i * 128)}}
		if _, err := p.Renderer.Draw(s, &p.Visual, dst); err != nil {
			t.Fatal(err)
		}
	}
	// Source cell44's opaque8c8c underlay closes theleftboundary. TarmacEC00
	// then writes EC on even parity. Both replace old0e markings; no clear.
	for _, v := range []struct {
		x, y int
		ink  byte
	}{{33, 0, 0x8c}, {34, 0, 0xec}, {179, 198, 0x8c}, {179, 199, 0xec}} {
		if got := dst[v.y*320+v.x]; got != v.ink {
			t.Fatalf("(%d,%d)=%02x expectedsource%02x", v.x, v.y, got, v.ink)
		}
	}
}
