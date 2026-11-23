package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	"testing"
)

func TestCockpitMapUsesSameCommittedRouteAsGPS(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	p := c.Rendering
	r := &p.Flight.Navigation[0]
	r.SetIndex(2)
	r.Raw[1], r.Raw[5] = 3, 0
	r.SetPoint(1, navigation.Point{H0: 200000, H1: 250000, Tail: 123})
	r.SetPoint(2, navigation.Point{H0: 203410, H1: 247725, Tail: 456})
	r.ToFlight(p.Flight.State)
	p.publishNavigation()
	p.publishAvionics()
	m := p.Cockpit.Map.Routes[0]
	if m.Index != 2 || m.Mode != 3 || m.Points[0].H0 != 200000 || m.Points[1].H1 != 247725 || m.H0 != p.Initialized.Player.H0 || m.H1 != p.Initialized.Player.H1 {
		t.Fatalf("map adapter disconnected from authoritative route: %+v", m)
	}
	before, _ := json.Marshal(p.Flight.Navigation)
	for n := 0; n < 3; n++ {
		p.Dirty = true
		c.Frame(false)
	}
	after, _ := json.Marshal(p.Flight.Navigation)
	if !bytes.Equal(before, after) {
		t.Fatal("cockpit map mutated route/tails/index per draw")
	}
}

func TestAcquiredMastBezelRoundTripsAllEligibleAircraft(t *testing.T) {
	for _, id := range []int{1, 3, 4} {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("aircraft%d-scale%d", id, scale), func(t *testing.T) {
				c := mastFixture(t, id, "land")
				jFinishFade(t, c)
				p := c.Rendering
				p.Scene.RenderScale = scale
				p.Frame = make([]byte, p.Scene.TargetPixels())
				p.worldFrame = make([]byte, p.Scene.TargetPixels())
				p.presentation.mode = "stepped"
				if err := c.Handle(Actions{ViewKey: 2}); err != nil {
					t.Fatal(err)
				}
				// Deliberately supplied acquired-state fixture; this is not a
				// claim that the photographed mission/target was reconstructed.
				target := 0
				for i, e := range p.Initialized.Entities {
					if i > 0 && e.ModelID > 0 {
						target = i + 1
						break
					}
				}
				if target == 0 {
					t.Fatal("source initialized fixture has no target model")
				}
				a := c.Office.Assets.Cockpit
				art := a.Sprites["mast-view"]
				stateBefore, _ := json.Marshal(p.Initialized)
				for _, acquired := range []int{0, target, 0, target} {
					p.Flight.Avionics.Target = uint16(acquired)
					p.Flight.Avionics.TargetPanelZoom = 512
					p.publishAvionics()
					p.resetPresentation()
					p.Dirty = true
					c.Frame(false)
					if p.Error != nil {
						t.Fatal(p.Error)
					}
					for y := 0; y < art.Height; y++ {
						for x := 0; x < art.Width; x++ {
							v := art.Pixels[y*art.Width+x]
							if v == 0 {
								continue
							}
							for sy := 0; sy < scale; sy++ {
								for sx := 0; sx < scale; sx++ {
									at := ((127+y)*scale+sy)*(320*scale) + (55+x)*scale + sx
									if p.Frame[at] != v {
										t.Fatalf("source bezel erased target%d sourcepixel%d,%d got%x want%x", acquired, x, y, p.Frame[at], v)
									}
								}
							}
						}
					}
					if err := c.Handle(Actions{ViewKey: 1}); err != nil {
						t.Fatal(err)
					}
					if err := c.Handle(Actions{ViewKey: 2}); err != nil {
						t.Fatal(err)
					}
				}
				stateAfter, _ := json.Marshal(p.Initialized)
				if !bytes.Equal(stateBefore, stateAfter) {
					t.Fatal("draw/view transition mutated mission entity/store state")
				}
			})
		}
	}
}

func TestMastCaptionUsesLiteralSourceTextAnchorsAtBothScales(t *testing.T) {
	a := &cockpit.Assets{Font: cockpit.Font{First: 32, Last: 126, Width: 6, Height: 6, Advances: bytes.Repeat([]byte{6}, 95), Rows: make([]byte, 95*6)}}
	for i := 0; i < 95; i++ {
		a.Font.Rows[i*6] = 0x80
	}
	a.ScenePresentation.ConfirmationRemap = make([]byte, 256)
	for _, scale := range []int{1, 2} {
		p := bytes.Repeat([]byte{99}, 64000*scale*scale)
		if err := targetCaption(p, render3d.Rect{Left: 69, Top: 132, Right: 251, Bottom: 196}, "A", 0, a, scale, 1000, 0, "B", true); err != nil {
			t.Fatal(err)
		}
		for _, xy := range [][2]int{{156, 180}, {156, 172}, {100, 180}, {200, 180}} {
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					if p[(xy[1]*scale+sy)*(320*scale)+xy[0]*scale+sx] != 31 {
						t.Fatalf("source mast caption scale%d at%+v", scale, xy)
					}
				}
			}
		}
		if p[(190*scale)*(320*scale)+100*scale] != 21 {
			t.Fatal("old primary-panel bottom-relative caption remains")
		}
	}
}
