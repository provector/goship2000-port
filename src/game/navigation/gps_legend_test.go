package navigation

import (
	"gunship2000-assets/game/cockpit"
	"reflect"
	"testing"
)

func TestGPSLegendsFollowContextAndUnavailableCommands(t *testing.T) {
	g := routeFixture()
	name, regions := g.legend()
	if name != "gps-controls-overview" || len(regions) != 4 || regions[0] != (legendRegion{50, 172, 32, 6, 10, 32}) {
		t.Fatal(name, regions)
	}
	g.Active[1] = true
	g.Editing = true
	_, regions = g.legend()
	if regions[0] != (legendRegion{87, 172, 10, 5, 10, 15}) {
		t.Fatal("F tag source0a→0f", regions)
	}
	g.Low = true
	name, _ = g.legend()
	if name != "gps-controls" {
		t.Fatal(name)
	}
	g.Selected = 1
	name, regions = g.legend()
	if name != "gps-controls-follower-detail" || regions[0].next != 32 {
		t.Fatal(name, regions)
	}
	g.Low = false
	name, _ = g.legend()
	if name != "gps-controls-follower-overview" {
		t.Fatal(name)
	}
	before := *g.Routes
	g.Editing = false
	for _, key := range []string{"f", "b", "c", "m", "w", "h", "g", "d", "a", "s", "l", "1", "2", "3"} {
		if _, err := g.Handle(Input{Key: key}); err != nil {
			t.Fatal("disabled action should not become error", key, err)
		}
	}
	if !reflect.DeepEqual(before, *g.Routes) || g.Editing {
		t.Fatal("unavailable follower orders mutated routes")
	}
}

func TestGPSLegendNewlineAndExactDisabledInk(t *testing.T) {
	a := &cockpit.Assets{Font: cockpit.Font{First: 32, Last: 126, Width: 1, Height: 6, Advances: make([]byte, 95), Rows: make([]byte, 95*6)}, Literals: map[string]string{"gps-controls-overview": "A\nB"}}
	for i := range a.Font.Advances {
		a.Font.Advances[i] = 1
		a.Font.Rows[i*6] = 0x80
	}
	g := routeFixture()
	dst := make([]byte, 64000)
	// Nearby non-ink source pixels must survive exact-index replacement.
	dst[172*320+51] = 14
	if err := g.drawLegend(dst, a, cockpit.Full); err != nil {
		t.Fatal(err)
	}
	if dst[172*320+50] != 32 || dst[178*320+4] != 32 || dst[178*320+50] != 0 || dst[172*320+51] != 14 {
		t.Fatal("source text-window LF or0a-only unavailable remap incorrect")
	}
}
