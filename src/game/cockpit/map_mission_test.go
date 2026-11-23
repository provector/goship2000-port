package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestMissionMarkerSourceGatesAndArgumentOperands(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original bytes")
	}
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct {
		ip  int
		hex string
	}{
		{0x2fd, "26833e6c1500"}, {0x410, "6a000e68df078b461080c402"},
		{0x443, "26a1f10a8e069e32263906bc157e5e"},
		{0x452, "6a000e68c4078b461080c402"},
		{0x4b5, "26833ee81500"}, {0x59f, "6a000e685a078b461080c402"},
		{0x5c7, "26833e261600"}, {0x6b1, "6a000e6822078b461080c402"},
	} {
		want, e := hex.DecodeString(v.hex)
		if e != nil || !bytes.Equal(b[0x1bb0+v.ip:0x1bb0+v.ip+len(want)], want) {
			t.Fatalf("original mission marker operand%04x", v.ip)
		}
	}
	if !bytes.Equal(b[0x487f0+0xaf1:0x487f0+0xaf3], []byte{0, 0}) {
		t.Fatal("source fresh FARP loss word")
	}
}

func TestMissionMarkerGatesOrderingShadowAndPurity(t *testing.T) {
	a := testAssets()
	a.Sine[1024] = 16384 // independently supplied identitycos for this arithmetic fixture
	for i, name := range []string{"map-07df", "map-07c4", "map-075a", "map-0722", "map-081e"} {
		a.MapVectors[name] = []Vector{{Colour: byte(10 + i), Points: [][2]int16{{0, 0}, {2, 0}}}}
	}
	project := func(h0, h1 uint32) (int, int) { return int(h0), int(h1) }
	m := MapMission{Base: MapObjective{Status: 1, H0: 20, H1: 20}, FARP: MapObjective{Status: 1, H0: 40, H1: 20}, Primary: MapObjective{Status: 2, H0: 60, H1: 20, EndpointH0: 1}, Secondary: MapObjective{Status: 3, H0: 80, H1: 20}, FARPCount: 2, FARPLosses: 1}
	before := m
	p := bytes.Repeat([]byte{9}, 64000)
	if e := a.DrawMissionMarkers(p, &m, project, -455, 8, Full); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct {
		x   int
		ink byte
	}{{20, 10}, {40, 11}, {60, 12}, {80, 13}} {
		if p[20*320+v.x] != v.ink || p[21*320+v.x+1] != 0 || p[21*320+v.x+3] != 0 || p[19*320+v.x] != 9 {
			t.Fatalf("source main/shadow/order around%+v", v)
		}
	}
	if before != m {
		t.Fatal("drawing mutated mission state")
	}
	// Exact signed Count>AF1, not presence alone; achieved/status0 hides objectives.
	for _, q := range []struct {
		count, loss int16
		visible     bool
	}{{2, 2, false}, {2, 3, false}, {2, 1, true}, {0, 0, false}, {-1, -2, true}} {
		clear(p)
		m = MapMission{FARP: before.FARP, FARPCount: q.count, FARPLosses: q.loss}
		if e := a.DrawMissionMarkers(p, &m, project, -455, 8, Full); e != nil {
			t.Fatal(e)
		}
		if (p[20*320+40] != 0) != q.visible {
			t.Fatalf("signed FARP boundary%+v", q)
		}
	}
	clear(p)
	m = before
	m.Base.Status = 0
	m.Primary.Status = 0
	m.Secondary.Status = 0
	m.FARP.Status = 0
	if e := a.DrawMissionMarkers(p, &m, project, -455, 8, Full); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(p, make([]byte, 64000)) {
		t.Fatal("zero status drew markers")
	}
}

func TestSourceCorridorMarkerArmsAndExactBoundaries(t *testing.T) {
	a := testAssets()
	a.MapVectors["map-07df"] = nil
	project := func(uint32, uint32) (int, int) { return 100, 100 }
	for _, v := range []struct {
		h0         uint32
		horizontal bool
	}{{0x7fff, false}, {0x8000, true}, {0x78000, true}, {0x78001, false}} {
		p := make([]byte, 64000)
		m := MapMission{Base: MapObjective{Status: 1, H0: v.h0}, PassThrough: true}
		if e := a.DrawMissionMarkers(p, &m, project, -3279, 8, Full); e != nil {
			t.Fatal(e)
		}
		// Source DIVS ±65536/-3279 truncates to ±19, inclusive line.
		if v.horizontal {
			if p[100*320+81] != 1 || p[100*320+119] != 1 || p[81*320+100] != 0 {
				t.Fatal("horizontal source arm", v)
			}
		} else {
			if p[81*320+100] != 1 || p[119*320+100] != 1 || p[100*320+81] != 0 {
				t.Fatal("vertical source arm", v)
			}
		}
		if p[100*320+80] != 0 || p[80*320+100] != 0 {
			t.Fatal("corridor span exceeded hand19 bound")
		}
	}
}
