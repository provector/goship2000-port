package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/navigation"
	gamestate "gunship2000-assets/game/state"
)

func TestGPSMissionAnchorsUseDisplayRecordDomainsAndFARPLosses(t *testing.T) {
	for _, family := range []string{"land", "carrier", "pass"} {
		c := mastFixture(t, 0, family)
		jFinishFade(t, c)
		p := c.Rendering
		if err := c.openGPS(); err != nil {
			t.Fatal(err)
		}
		g := p.GPS
		if g.Mission == nil || p.Cockpit.Map.Mission == nil {
			t.Fatal("source mission producer remains disconnected")
		}
		if g.Mission == p.Cockpit.Map.Mission {
			t.Fatal("GPS did not take an immutable mission snapshot")
		}
		for _, v := range []struct {
			role   string
			actual cockpit.MapObjective
		}{{"base", g.Mission.Base}, {"FARP", g.Mission.FARP}, {"primary", g.Mission.Primary}, {"secondary", g.Mission.Secondary}} {
			o, ok := c.Mission.Data.Objectives[v.role]
			if !ok {
				continue
			}
			if v.actual.Status != int16(o.DisplayKind) || v.actual.H0 != uint32(o.Point.X) || v.actual.H1 != uint32(o.Point.Y) {
				t.Fatalf("%s/%s used raw kind, original point or live entity %+v %+v", family, v.role, o, v.actual)
			}
		}
		if g.Mission.PassThrough != (p.Initialized.Switches&0x800 != 0) {
			t.Fatal("pass-through inferred from labels instead of launch bit0800")
		}
		for _, low := range []bool{false, true} {
			g.Low = low
			g.Center = g.Poses[0]
			mission := g.Mission
			g.Mission = nil
			writeGPSCapture(t, c, fmt.Sprintf("actual-%s-%t-before-omission-control", family, low), c.gpsFrame())
			g.Mission = mission
			writeGPSCapture(t, c, fmt.Sprintf("actual-%s-%t-after", family, low), c.gpsFrame())
		}
		c.closeGPS()
		p.Initialized.Sortie = &gamestate.SortieState{FARPLosses: 5}
		if err := c.openGPS(); err != nil {
			t.Fatal(err)
		}
		if p.GPS.Mission.FARPLosses != 5 {
			t.Fatal("GPS failed to read current transient FARP loss owner")
		}
		before, _ := json.Marshal(struct {
			I any
			N any
			A any
		}{p.Initialized, p.Flight.Navigation, p.Flight.Avionics})
		for i := 0; i < 4; i++ {
			c.Frame(false)
		}
		after, _ := json.Marshal(struct {
			I any
			N any
			A any
		}{p.Initialized, p.Flight.Navigation, p.Flight.Avionics})
		if !bytes.Equal(before, after) {
			t.Fatal("mission marker drawing mutated flight/navigation/known contacts")
		}
	}
}

func TestObjectiveDirectionUsesSourceShiftAndArgumentOrder(t *testing.T) {
	c := mastFixture(t, 0, "land")
	p := c.Rendering
	for _, v := range []struct {
		dx, dy int32
		angle  uint16
	}{{64, 0, 0xc000}, {0, 64, 0}, {-64, 0, 0x4000}, {0, -64, 0x8000}, {-1, 0, 0x4000}} {
		m := *c.Mission.Data
		m.Objectives = map[string]gamestate.MissionObjective{"primary": {RawKind: 7, DisplayKind: 2, Point: gamestate.MissionPoint{X: 100000, Y: 100000}, Endpoint: &gamestate.MissionPoint{X: 100000 + v.dx, Y: 100000 + v.dy}}}
		p.initializeNavigationObjectives(&m)
		q := p.Cockpit.Map.Mission.Primary
		if q.Status != 2 || q.EndpointDirection != v.angle {
			t.Fatalf("sourceSAR6/callee(-dx,dz) %+v=>%04x", v, q.EndpointDirection)
		}
	}
}

func TestMissionMarkersUseEachAircraftMapPanelClip(t *testing.T) {
	c := mastFixture(t, 0, "land")
	a := c.Office.Assets.Cockpit
	for id := 0; id < 8; id++ {
		family, e := cockpit.Family(id)
		if e != nil {
			t.Fatal(e)
		}
		d := a.Definitions[family]
		for _, secondary := range []bool{false, true} {
			base := 0x54
			if secondary {
				base = 0x60
			}
			left, top := int(d.Parameters[base/2]), int(d.Parameters[(base+2)/2])
			right, bottom := int(d.Parameters[(base+4)/2]), int(d.Parameters[(base+6)/2])
			if left >= right || top >= bottom { // No supplied panel in this CPIT definition.
				continue
			}
			in := cockpit.MapInputs{Grid: make([]byte, 4096), Atlas: bytes.Repeat([]byte{4}, 192*144), H0: 0x40000, H1: 0x40000}
			before, after := bytes.Repeat([]byte{9}, 64000), bytes.Repeat([]byte{9}, 64000)
			if e := a.DrawMap(before, d, secondary, in); e != nil {
				t.Fatal(id, secondary, e)
			}
			in.Mission = &cockpit.MapMission{Primary: cockpit.MapObjective{Status: 1, H0: in.H0, H1: in.H1}}
			if e := a.DrawMap(after, d, secondary, in); e != nil {
				t.Fatal(id, secondary, e)
			}
			if bytes.Equal(before, after) {
				t.Fatal("mission tail missing from real CPIT panel", id, secondary)
			}
			for y := 0; y < 200; y++ {
				for x := 0; x < 320; x++ {
					if (x < left || x >= right || y < top || y >= bottom) && before[y*320+x] != after[y*320+x] {
						t.Fatal("marker escaped source panel clip", id, secondary, x, y)
					}
				}
			}
		}
	}
}

func TestGPSMissionOverlaysAllPreparedWorlds(t *testing.T) {
	c := mastFixture(t, 0, "land")
	a := c.Office.Assets.Cockpit
	root := os.Getenv("GS2000_L3_GPS_CAPTURE")
	// Deliberately supplied marker fixture exercises carrier/projection domains;
	// it is not a naturally generated mission in each of the14 worlds.
	fixture := cockpit.MapMission{Base: cockpit.MapObjective{Status: 1, H0: 252144, H1: 262144}, FARP: cockpit.MapObjective{Status: 1, H0: 270144, H1: 254144}, Primary: cockpit.MapObjective{Status: 2, H0: 270144, H1: 262144}, Secondary: cockpit.MapObjective{Status: 3, H0: 262144, H1: 270144}, FARPCount: 1}
	for id, w := range c.Rendering.Renderer.Assets.Worlds {
		var routes [5]navigation.Route
		g := navigation.New(&routes, [5]navigation.Point{}, [5]bool{})
		for _, low := range []bool{false, true} {
			g.Low = low
			g.Center = navigation.Point{H0: 262144, H1: 262144}
			before, after := make([]byte, 64000), make([]byte, 64000)
			g.Mission = nil
			if e := g.Draw(before, w, a); e != nil {
				t.Fatal(id, e)
			}
			g.Mission = &fixture
			if e := g.Draw(after, w, a); e != nil {
				t.Fatal(id, e)
			}
			if bytes.Equal(before, after) {
				t.Fatal("omitted mission overlay", id, low)
			}
			state, _ := json.Marshal(g)
			again := make([]byte, 64000)
			if e := g.Draw(again, w, a); e != nil {
				t.Fatal(e)
			}
			stateAfter, _ := json.Marshal(g)
			if !bytes.Equal(after, again) || !bytes.Equal(state, stateAfter) {
				t.Fatal("draw-dependent marker state", id, low)
			}
			if root != "" {
				// Use this world's original DAY context only for diagnostic colours.
				c.Rendering.Scene.World = id
				if pal, e := c.Rendering.Renderer.Palette(c.Rendering.Scene, &c.Rendering.Visual); e == nil {
					c.Rendering.Palette = pal
				}
				name := fmt.Sprintf("%s-%t", id, low)
				writeGPSCapture(t, c, name+"-before-omission-control", before)
				writeGPSCapture(t, c, name+"-after", after)
			}
		}
	}
}

func writeGPSCapture(t *testing.T, c *FrontEnd, name string, pixels []byte) {
	t.Helper()
	root := os.Getenv("GS2000_L3_GPS_CAPTURE")
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(root, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	pic := image.NewPaletted(image.Rect(0, 0, 320, 200), c.CurrentPalette())
	copy(pic.Pix, pixels)
	if err = png.Encode(f, pic); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, name+".indices"), pixels, 0600); err != nil {
		t.Fatal(err)
	}
}
