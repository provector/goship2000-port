package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/navigation"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGPSActualSceneModalRouteAndNoCatchup(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	c.Handle(Actions{})
	for _, fps := range []int{30, 60, 144} {
		// Nonzero timer/signature witnesses distinguish a real freeze from
		// merely observing already-expired default defensive state.
		p.Flight.Defenses.ChaffTimer = 9
		p.Flight.Defenses.FlareTimer = 7
		p.Flight.Defenses.IRActive = 15
		p.Flight.Defenses.IRCharge = 15
		p.Flight.Defenses.RadarSignature = 6
		if err := c.Handle(Actions{GPSOpen: true}); err != nil {
			t.Fatal(err)
		}
		if p.GPS == nil {
			t.Fatal("F10 normalmanagerdidnotopen")
		}
		before := *p.Flight.State
		fuel := append([]int16(nil), p.Flight.Systems.Fuel...)
		steps := p.Flight.Steps
		visual := p.Visual.Phase
		rng := p.Initialized.RandomState
		defense := p.Flight.Defenses
		avionics, _ := json.Marshal(p.Flight.Avionics)
		for n := 0; n < fps*7; n++ {
			if err := c.AdvanceRendering(1 / float64(fps)); err != nil {
				t.Fatal(err)
			}
			c.Frame(false)
		}
		if !reflect.DeepEqual(before, *p.Flight.State) || !reflect.DeepEqual(fuel, p.Flight.Systems.Fuel) || steps != p.Flight.Steps || visual != p.Visual.Phase || rng != p.Initialized.RandomState {
			t.Fatalf("GPS at%dFPSadvancedstate", fps)
		}
		afterAvionics, _ := json.Marshal(p.Flight.Avionics)
		if defense != p.Flight.Defenses || !bytes.Equal(avionics, afterAvionics) {
			t.Fatal("seven-second GPS modal advanced defense/contact state")
		}
		// Source edit begins by clearing coordinates but preserving tails/opaque bytes.
		raw := p.Flight.Navigation[0]
		c.Handle(Actions{GPSKey: "f"})
		if !p.GPS.Editing {
			t.Fatal("Fdidnotedit")
		}
		c.Handle(Actions{Inside: true, X: 112, Y: 80, Selector1: true})
		q := p.Flight.Navigation[0].Point(1)
		if q.H0 != 257510 || q.H1 != 268878 {
			t.Fatalf("actualpointerplacement%+v", q)
		}
		c.Handle(Actions{Cancel: true})
		if p.GPS == nil || p.GPS.Editing || p.Flight.Navigation[0] != raw {
			t.Fatal("Esc mustrollbackwithoutleave")
		}
		c.Handle(Actions{GPSKey: "f"})
		c.Handle(Actions{Inside: true, X: 112, Y: 80, Selector1: true})
		partial := p.Flight.Navigation[0]
		c.Handle(Actions{ViewKey: 1})
		if p.GPS != nil || p.Flight.Navigation[0] != partial || p.Flight.State.WaypointIndex != partial.Index() {
			t.Fatal("F1 earlyexitlostprovisionaldata")
		}
		if p.Flight.Clock.InterpolationFraction() != 0 {
			t.Fatal("GPSreturncatchupphase")
		}
		// One small display interval after return cannot repay the one-second modal.
		c.Handle(Actions{})
		c.AdvanceRendering(.001)
		if p.Flight.Steps != steps {
			t.Fatal("catchupburst")
		}
		p.Flight.Navigation[0] = raw
		raw.ToFlight(p.Flight.State)
	}
}
func TestGPSActualPaletteAndPreparedOverview(t *testing.T) {
	c := mastFixture(t, 2, "land")
	jFinishFade(t, c)
	p := c.Rendering
	p.Initialized.Switches |= 0x200
	p.PaletteState.Toggle(p.Initialized.Switches)
	before := append([]byte(nil), p.PaletteState.RGB6()...)
	c.Handle(Actions{})
	c.Handle(Actions{GPSOpen: true})
	if p.GPS == nil {
		t.Fatal("GPSmissing")
	}
	c.Handle(Actions{ViewKey: 1, ViewShift: true})
	if p.GPS == nil {
		t.Fatal("ShiftF1 notsourceGPSexitkey")
	}
	c.Handle(Actions{NightVisionToggle: true})
	if !bytes.Equal(before, p.PaletteState.RGB6()) {
		t.Fatal("AltTinsideGPStoggledselectedbank")
	}
	if dir := os.Getenv("GS2000_L_GPS_CAPTURE"); dir != "" {
		mastCapture(t, c, dir, "gps-high")
	}
	c.Handle(Actions{GPSKey: "space"})
	frame := c.gpsFrame()
	if p.Error != nil {
		t.Fatal(p.Error)
	}
	if len(frame) != 64000 || len(p.Renderer.Assets.Worlds[p.Scene.World].Overview) != 89*65 {
		t.Fatal("missingtypedSMP")
	}
	if dir := os.Getenv("GS2000_L_GPS_CAPTURE"); dir != "" {
		mastCapture(t, c, dir, "gps-detail")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "gps-detail.indices"), frame, 0600); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(p.GPS, "", "  ")
		os.WriteFile(filepath.Join(dir, "gps-state.json"), raw, 0600)
	}
	c.Handle(Actions{Cancel: true})
	if p.GPS != nil || !bytes.Equal(before, p.PaletteState.RGB6()) {
		t.Fatal("GPSreturnlostnightbank")
	}
}
func TestGPSHomeSourceDomains(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	c.Handle(Actions{})
	c.openGPS()
	g := p.GPS
	home := p.Initialized.Rows[0].Point()
	if g.Home.H0 != uint32(home.X) || g.Home.H1 != uint32(home.Y) {
		t.Fatal("homeusesliveposeorbaseoverride")
	}
	if o, ok := c.Mission.Data.Objectives["FARP"]; ok {
		if g.FARPPresent != (o.DisplayKind != 0) {
			t.Fatal("presence mustDisplayKind notcount")
		}
	}
	r := &p.Flight.Navigation[0]
	tail := r.Point(1).Tail
	g.Handle(navigation.Input{Key: "b"})
	if r.Point(1).Tail != tail || r.Index() != 1 {
		t.Fatal("Bmustpreservetailandsel1")
	}
}

func TestGPSActualNavigationMFDAircraftFamilies(t *testing.T) {
	for aircraft := 0; aircraft < 8; aircraft++ {
		c := mastFixture(t, aircraft, "land")
		jFinishFade(t, c)
		p := c.Rendering
		r := &p.Flight.Navigation[0]
		r.SetIndex(1)
		r.SetPoint(1, navigation.Point{H0: p.Flight.State.Pose.H0 + 1600, H1: p.Flight.State.Pose.H1, Tail: 0x73})
		r.ToFlight(p.Flight.State)
		p.publishNavigation()
		if !p.Cockpit.Navigation.Active || p.Cockpit.Navigation.Bearing != 0x4000 || p.Cockpit.Route.Points[0].Range != 200 {
			t.Fatalf("aircraft%d producerdomain %+v", aircraft, p.Cockpit.Route)
		}
		before, _ := json.Marshal(p.Initialized)
		for _, mode := range []int16{1, 2, 3} {
			p.Cockpit.PrimaryMFD = mode
			p.Cockpit.SecondaryMFD = mode
			p.Cockpit.PrimaryMap = false
			p.Cockpit.SecondaryMap = false
			p.Dirty = true
			c.Frame(false)
			if p.Error != nil {
				t.Fatalf("aircraft%d MFD%d:%v", aircraft, mode, p.Error)
			}
		}
		after, _ := json.Marshal(p.Initialized)
		if !bytes.Equal(before, after) {
			t.Fatal("MFDdrawingmutatedauthoritativestate")
		}
	}
}

func TestGPSActualReviewedCursorAndSelectorPaths(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	c.Handle(Actions{})
	c.Handle(Actions{GPSOpen: true})
	c.Handle(Actions{GPSKey: "f"})
	if p.GPS == nil || !p.GPS.Editing {
		t.Fatal("GPS route owner missing")
	}
	frame := c.gpsFrame()
	if p.Error != nil {
		t.Fatal(p.Error)
	}
	x, y := p.GPS.CursorX, p.GPS.CursorY
	// Source4:2144's independent literal98h endpoints±2 become±4 at style0107.
	for n := -4; n <= 4; n++ {
		if frame[y*320+x+n] != 0x98 || frame[(y+n)*320+x] != 0x98 {
			t.Fatal("actual prepared GPS uses wrong cursor artwork/palette")
		}
	}
	c.Handle(Actions{Inside: true, X: 112, Y: 80, Selector1: true})
	c.Handle(Actions{GPSKey: "f"})
	committed := p.Flight.Navigation[0]
	c.Handle(Actions{Cancel: true})
	if p.GPS != nil || p.Flight.Navigation[0] != committed {
		t.Fatal("finish then Escape must leave completed route")
	}
	c.Handle(Actions{})
	c.Handle(Actions{GPSOpen: true})
	c.Handle(Actions{GPSKey: "f"})
	c.Handle(Actions{Cancel: true})
	if p.GPS == nil || p.GPS.Editing || p.Flight.Navigation[0] != committed {
		t.Fatal("new F command Escape must restore its completed-route snapshot")
	}
	state := *p.Flight.State
	c.Handle(Actions{Selector2: true})
	if p.GPS != nil || *p.Flight.State != state {
		t.Fatal("native Selector2 exit changed flight state")
	}

}
