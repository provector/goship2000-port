package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"reflect"
	"testing"
	"time"
)

func TestAvionicsInputEdgesAndModifierIsolation(t *testing.T) {
	var l inputLatch
	b := Buttons{DefenseKeys: [4]bool{true, true, true, true}, GPSOpen: true, WaypointKeys: [2]bool{true, true}, GPSKey: "f"}
	a := l.Read(b)
	if len(a.DefenseCommands) != 4 || !a.GPSOpen || !a.WaypointNext || !a.WaypointPrevious || a.GPSKey != "f" {
		t.Fatal(a)
	}
	a = l.Read(b)
	if len(a.DefenseCommands) != 0 || a.GPSOpen || a.WaypointNext || a.WaypointPrevious || a.GPSKey != "" {
		t.Fatal("held replay", a)
	}
	l.Read(Buttons{})
	a = l.Read(b)
	if len(a.DefenseCommands) != 4 || !a.GPSOpen {
		t.Fatal("repress", a)
	}
}
func TestDefenseLogicalOwnershipPauseAndGPS(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	r := p.Flight
	c.Message = ""
	r.Defenses = flight.NewDefenses(1, 2)
	c.Handle(Actions{})
	if e := c.Handle(Actions{DefenseCommands: []int{0, 1, 2, 3}, DefenseHeldValid: true, DefenseHeld: [2]bool{true, true}}); e != nil {
		t.Fatal(e)
	}
	if r.Defenses.Chaff != 1 {
		t.Fatal("input advanced state beforelogicaltick")
	}
	if e := c.AdvanceRendering(.1); e != nil {
		t.Fatal(e)
	}
	if r.Defenses.Chaff != -2 || r.Defenses.Flares != -1 || r.Defenses.RadarSignature < 1 || r.Defenses.RadarSignature > 8 || r.Defenses.IRActive == 0 {
		t.Fatalf("source defenses %+v", r.Defenses)
	}
	before := r.Defenses
	contact, _ := json.Marshal(r.Avionics)
	rng := p.Initialized.RandomState
	for i := 0; i < 10; i++ {
		p.Dirty = true
		c.Frame(false)
	}
	after, _ := json.Marshal(r.Avionics)
	if before != r.Defenses || !bytes.Equal(contact, after) || rng != p.Initialized.RandomState {
		t.Fatal("draw advancedsystems")
	}
	// Native momentary jammers release on a modal transition; counters/stores freeze.
	if before.IRActive != 0 {
		before.IRCharge = before.IRActive
	}
	before.IRActive = 0
	before.RadarSignature = 0
	c.Handle(Actions{FlightPause: true})
	if e := c.AdvanceRendering(1); e != nil {
		t.Fatal(e)
	}
	if before != r.Defenses {
		t.Fatal("pause advanceddefenses")
	}
	c.Handle(Actions{AnyKey: true})
	c.Handle(Actions{})
	if e := c.Handle(Actions{GPSOpen: true}); e != nil {
		t.Fatal(e)
	}
	if p.GPS == nil {
		t.Fatal("GPSnotopened")
	}
	c.Handle(Actions{FlightFocusLost: true})
	c.Handle(Actions{GPSKey: "f"})
	if p.GPS.Editing {
		t.Fatal("focusreacquisitionactivatededit")
	}
	c.Handle(Actions{})
	before = r.Defenses
	steps := r.Steps
	pose := r.State.Pose
	rng = p.Initialized.RandomState
	for i := 0; i < 12; i++ {
		if e := c.AdvanceRendering(.2); e != nil {
			t.Fatal(e)
		}
		c.Handle(Actions{DefenseCommands: []int{0, 1}, NightVisionToggle: true})
		c.Frame(false)
	}
	if before != r.Defenses || steps != r.Steps || pose != r.State.Pose || rng != p.Initialized.RandomState {
		t.Fatal("GPSdidnotpauseauthoritativeowners")
	}
	c.Handle(Actions{Cancel: true})
	if p.GPS != nil {
		t.Fatal("GPSdidnotexit")
	}
	c.Handle(Actions{})
	c.AdvanceRendering(.01)
	if r.Steps != steps {
		t.Fatal("GPSreturncatchupburst")
	}
}
func TestHighResolutionGPSModalAndPreparedSession(t *testing.T) {
	c := mastFixture(t, 2, "pass")
	jFinishFade(t, c)
	// The fixture already started a flight. Close its spool before replacing
	// the session to exercise the higher-resolution launch.
	c.AbandonFilmRecording()
	c.Runtime.SimulationRenderScale = 2
	if e := c.enterRenderingPreview(c.Mission.Final); e != nil {
		t.Fatal(e)
	}
	c.fade = nil
	p := c.Rendering
	c.Message = ""
	img, e := c.RenderRGBA(false)
	if e != nil {
		t.Fatal(e)
	}
	if img.Rect.Dx() != 640 || img.Rect.Dy() != 400 {
		t.Fatal(img.Rect)
	}
	before, _ := json.Marshal(c.Mission.Data)
	saved := c.Store.State()
	c.Handle(Actions{})
	c.Handle(Actions{GPSOpen: true})
	img, e = c.RenderRGBA(false)
	if e != nil {
		t.Fatal(e)
	}
	if img.Rect.Dx() != 320 || p.GPS == nil {
		t.Fatal("GPSlogicaltarget")
	}
	c.Handle(Actions{GPSKey: "f"})
	c.Handle(Actions{Selector1: true, Inside: true, X: 130, Y: 90})
	provisional := p.Flight.Navigation[0]
	c.Handle(Actions{ViewKey: 1})
	if p.GPS != nil || provisional != p.Flight.Navigation[0] {
		t.Fatal("Fkeywrite-through")
	}
	img, e = c.RenderRGBA(false)
	if e != nil {
		t.Fatal(e)
	}
	if img.Rect.Dx() != 640 {
		t.Fatal("targetnotrestored")
	}
	view := *p.ViewControls
	c.Handle(Actions{Quit: true})
	img, e = c.RenderRGBA(false)
	if e != nil {
		t.Fatal(e)
	}
	if img.Rect.Dx() != 640 {
		t.Fatal("modalresolution")
	}
	c.Handle(Actions{Cancel: true})
	if !reflect.DeepEqual(*p.ViewControls, view) {
		t.Fatal("modalchangedview")
	}
	after, _ := json.Marshal(c.Mission.Data)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(saved, c.Store.State()) {
		t.Fatal("navigation/presentationchangedmissionorsave")
	}
	c.Handle(Actions{EndMission: true})
	c.Handle(Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || !reflect.DeepEqual(saved, c.Store.State()) {
		t.Fatal("endpreviewoutcome/save")
	}
}

func TestGPSHeldKeyboardHasSeparateSourceAxisOwner(t *testing.T) {
	for _, fps := range []int{30, 60, 144} {
		c := mastFixture(t, 0, "pass")
		c.Runtime = configuration.Defaults()
		c.Runtime.GPSOverviewKeyboardDivisor = 1 // source-speed reference remains selectable.
		jFinishFade(t, c)
		c.Handle(Actions{})
		c.Handle(Actions{GPSOpen: true})
		p := c.Rendering
		if p.GPS == nil {
			t.Fatal("GPSmissing")
		}
		pose := p.Flight.State.Pose
		rng := p.Initialized.RandomState
		c.Handle(Actions{FlightPause: true, NightVisionToggle: true})
		if p.Flight.Paused {
			t.Fatal("flightpauseescapedGPSmodal")
		}
		ticks := 0
		for phase := 0; phase < 2; phase++ {
			held := [4]bool{}
			if phase == 0 {
				held[1] = true
			}
			c.Handle(Actions{SceneDirections: held})
			for frame := 1; ; frame++ {
				elapsed := min(time.Duration(frame)*time.Second/time.Duration(fps), 100*time.Millisecond)
				total := time.Duration(phase)*100*time.Millisecond + elapsed
				goal := int(int64(total) * 70 / int64(time.Second))
				for ticks < goal {
					if e := c.Tick(); e != nil {
						t.Fatal(e)
					}
					ticks++
				}
				c.Frame(false)
				if elapsed == 100*time.Millisecond {
					break
				}
			}
		}
		// Seven held polls16..112:1+2+3+4+5+6+7=28px. Release56,28,..0 adds3+1=4.
		if p.GPS.CursorX != 144 || p.GPS.CursorY != 80 || p.Flight.gpsAxes.X != 0 {
			t.Fatalf("fps%d cursor%d,%d axes%+v", fps, p.GPS.CursorX, p.GPS.CursorY, p.Flight.gpsAxes)
		}
		if p.Flight.State.Pose != pose || p.Initialized.RandomState != rng || p.Flight.Axes.X != 0 {
			t.Fatal("GPScontrolledflight")
		}
	}
}
