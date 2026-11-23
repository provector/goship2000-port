package navigation

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSupportInterlocksRetainHistoricalModePolicy(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, h := range map[int]string{0xba4: "f6866aff02", 0xe02: "80be6aff00", 0x1000: "80be6aff00", 0x14a0: "80be6aff00", 0x119c: "8a46ae986bd81e", 0x3ce: "817eb42039", 0x3d8: "83769a01"} {
		v, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0x24290+ip:][:len(v)], v) {
			t.Fatalf("source mode consumer%x", ip)
		}
	}
	for _, source := range []bool{false, true} {
		for _, key := range []string{"f", "n", "b"} {
			g := routeFixture()
			g.Active[1] = true
			g.EnableSupport(0x31, 0, 0)
			before, _ := json.Marshal(g.ReplayState())
			if source {
				g.UseSourceSupportInterlocks()
			}
			after, _ := json.Marshal(g.ReplayState())
			if !bytes.Equal(before, after) {
				t.Fatal("policy added mutable checkpoint field")
			}
			g.Handle(Input{Key: "m"})
			route := g.Routes[0]
			selected := g.Selected
			g.Handle(Input{Key: key, AxisX: 16})
			if source {
				if g.Editing || g.Selected != selected || g.Routes[0] != route || !g.Support.Editing {
					t.Fatal("source mode interlock", key, g)
				}
			} else if key == "f" && !g.Editing || key == "n" && g.Selected == selected {
				t.Fatal("historical v4 policy silently upgraded", key)
			}
		}
	}
	// C is *not* blocked by FF6A in source119C. Its outer SHIP/mass producer
	// stays separate; no speculative blanket modal-input suppression is added.
	g := routeFixture()
	g.UseSourceSupportInterlocks()
	g.EnableSupport(0x31, 0, 0)
	g.Handle(Input{Key: "m"})
	if _, e := g.Handle(Input{Key: "c"}); e != nil || !g.Support.Editing {
		t.Fatal("C handling changed support mode", e)
	}
}

func TestNativeSpacePriorityRetainsHistoricalGPSInput(t *testing.T) {
	for _, support := range []bool{false, true} {
		for _, source := range []bool{false, true} {
			g := routeFixture()
			if source {
				g.UseSourceSupportInterlocks()
			}
			if support {
				g.EnableSupport(0x31, 0, 0)
				g.Handle(Input{Key: "m"})
			}
			before := g.CursorPoint()
			exit, e := g.Handle(Input{Key: "space", Selector1: true, AxisX: 16})
			if e != nil {
				t.Fatal(e)
			}
			if source {
				if exit != 0 || !g.Low || g.TakeSupportCommand() != nil {
					t.Fatal("Space leaked selector or lost source zoom priority", support, exit, g)
				}
				if support && g.Center.H0 != before.H0+512 {
					t.Fatal("source zoom then detail-axis order", g.Center, before)
				}
			} else if support {
				if g.TakeSupportCommand() == nil || g.Low {
					t.Fatal("v4 Support/Space selector interpretation changed")
				}
			} else if exit != -1 {
				t.Fatal("v4 ordinary selector exit changed", exit)
			}
		}
	}
}
