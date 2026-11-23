package cockpit

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestBaseMarkerLiteralArtworkAndTransparentInterior(t *testing.T) {
	// Original015b:07df, not a replacement glyph drawn from visual intuition.
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original byte verification")
	}
	if e != nil {
		t.Fatal(e)
	}
	want := []byte{1, 4, 253, 253, 253, 3, 3, 3, 3, 253, 253, 253, 15, 5, 255, 254, 255, 2, 1, 1, 255, 0, 1, 255, 255, 254, 0}
	if !bytes.Equal(b[0x1bb0+0x7df:0x1bb0+0x7df+len(want)], want) {
		t.Fatal("base vector source changed")
	}
	a := testAssets()
	a.Sine[1024] = 16384
	a.MapVectors["map-07df"] = []Vector{
		{Colour: 1, Points: [][2]int16{{-3, -3}, {-3, 3}, {3, 3}, {3, -3}, {-3, -3}}},
		{Colour: 15, Points: [][2]int16{{-1, -2}, {-1, 2}, {1, 1}, {-1, 0}, {1, -1}, {-1, -2}}},
	}
	p := bytes.Repeat([]byte{9}, 64000)
	m := MapMission{Base: MapObjective{Status: 3, H0: 160, H1: 100}}
	if e = a.DrawMissionMarkers(p, &m, func(x, y uint32) (int, int) { return int(x), int(y) }, -3279, 8, Full); e != nil {
		t.Fatal(e)
	}
	count := 0
	for y := -3; y <= 3; y++ {
		for x := -3; x <= 3; x++ {
			if x == -3 || x == 3 || y == -3 || y == 3 {
				if p[(100+y)*320+160+x] != 1 {
					t.Fatalf("literal palette1 border(%d,%d)", x, y)
				}
				count++
			}
		}
	}
	if count != 24 {
		t.Fatal("hand7x7square perimeter")
	}
	for y := -2; y <= 2; y++ {
		if p[(100+y)*320+159] != 15 {
			t.Fatal("source white B stem", y)
		}
	}
	if p[99*320+162] != 9 {
		t.Fatal("vector rendering incorrectly filled background")
	}
	if p[100*320+156] != 9 || p[100*320+164] != 0 {
		t.Fatal("source(+1,+1) shadow/outside ownership changed")
	}
}

func TestComancheSecondaryFiveIsSourceThreatDispatch(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional source bytes")
	}
	if e != nil {
		t.Fatal(e)
	}
	// Dispatch INC makes AD9=5 table slot6. This reaches1c9e, tests
	// damage05ff.bit40 and calls3586 threat display, never primary191f.
	if !bytes.Equal(b[0x1dc60+0x1cca:0x1dc60+0x1ccc], []byte{0x9e, 0x1c}) || !bytes.Equal(b[0x1dc60+0x1ca2:0x1dc60+0x1ca9], []byte{0x26, 0xf6, 0x06, 0xff, 0x05, 0x40, 0x75}) || !bytes.Equal(b[0x1dc60+0x1caa:0x1dc60+0x1cae], []byte{0x0e, 0xe8, 0xd8, 0x18}) {
		t.Fatal("secondary source branch changed")
	}
	for _, id := range []int{6, 7} {
		if family, e := Family(id); e != nil || family != "COM" {
			t.Fatal("Comanche aliases diverged", id, family, e)
		}
	}
}
