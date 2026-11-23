package menus

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func TestInitializedSceneViewsUseWindowEdgesAndKeepMission(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	p := c.Rendering
	if p.Initialized == nil || c.Mission.Initialized != p.Initialized || p.Inspection {
		t.Fatal("normal path has no authoritative initialization")
	}
	before, _ := json.Marshal(c.Mission.Data)
	entities, _ := json.Marshal(p.Initialized)
	saved := c.Store.State()
	var w windowInput
	viewport := officeViewport(960, 720, false)
	feed := func(b Buttons) {
		t.Helper()
		if e := c.Handle(w.ReadViewport(b, viewport)); e != nil {
			t.Fatal(e)
		}
	}
	key := func(k int) { t.Helper(); b := Buttons{}; b.FunctionKeys[k-1] = true; feed(b); feed(Buttons{}) }
	key(3)
	if p.ViewControls.Mode != 1 || p.Cockpit.CameraMode != 1 || p.Scene.Camera.H0 != p.Initialized.Player.H0 || p.Scene.Camera.H1 != p.Initialized.Player.H1 || p.Scene.Camera.Angles[1] != p.Initialized.Player.Angles[1]+0x4000 {
		t.Fatal("F3 identity/formula")
	}
	key(4)
	if p.ViewControls.Mode != 2 {
		t.Fatal("F4")
	}
	key(5)
	if p.ViewControls.Mode != 3 {
		t.Fatal("F5")
	}
	key(7)
	if p.ViewControls.Mode != 5 {
		t.Fatal("F7")
	}
	cam := *p.ViewControls
	key(6)
	key(8)
	if !reflect.DeepEqual(cam, *p.ViewControls) {
		t.Fatal("missing target fabricated a view")
	}
	key(1)
	if p.Scene.Camera.Height != p.Initialized.Player.Height+32 {
		t.Fatal("ordinary camera relative height")
	}
	feed(Buttons{AltS: true})
	feed(Buttons{AltS: true})
	if p.ViewControls.Arrangement != 1 {
		t.Fatal("held AltS repeated")
	}
	feed(Buttons{})
	feed(Buttons{AltQ: true})
	if c.Screen != ScreenQuit {
		t.Fatal("scene quit missing")
	}
	frozen := *p.ViewControls
	b := Buttons{}
	b.FunctionKeys[4] = true
	feed(b) // Key dismisses modal, is not a view action.
	if c.Screen != ScreenScene || !reflect.DeepEqual(frozen, *p.ViewControls) {
		t.Fatal("modal key leaked")
	}
	feed(b)
	if !reflect.DeepEqual(frozen, *p.ViewControls) {
		t.Fatal("held dismissal became a camera action")
	}
	feed(Buttons{})
	feed(Buttons{AltE: true})
	feed(Buttons{})
	feed(Buttons{Escape: true})
	feed(Buttons{})
	if c.Screen != ScreenScene || !reflect.DeepEqual(frozen, *p.ViewControls) {
		t.Fatal("End cancellation changed view")
	}
	after, _ := json.Marshal(c.Mission.Data)
	nowEntities, _ := json.Marshal(p.Initialized)
	if !bytes.Equal(before, after) || !bytes.Equal(entities, nowEntities) || !reflect.DeepEqual(saved, c.Store.State()) {
		t.Fatal("cameras changed authoritative preparation/save")
	}
	for role, ref := range p.Initialized.Objectives {
		o := c.Mission.Data.Objectives[role]
		if ref.SourceRow != o.ObjectIndex || ref.EntityID != p.Initialized.Entities[o.ObjectIndex].ID {
			t.Fatal("objective entity linkage", role)
		}
	}
}

func TestInitializedMastModalDropsStaleHeldInput(t *testing.T) {
	c := renderingController(t)
	// Source-default Longbow equipment is a supplied test input, not a cockpit
	// override. Its actual serialized selector admits the original mast guard.
	row, err := gamestate.InitializeReadinessRow(c.Office.Assets.Missions, c.Mission.Data, 1)
	if err != nil {
		t.Fatal(err)
	}
	c.Mission.ReadinessRows[0] = row
	startRendering(t, c)
	var w windowInput
	view := officeViewport(960, 720, false)
	feed := func(b Buttons) {
		t.Helper()
		if err := c.Handle(w.ReadViewport(b, view)); err != nil {
			t.Fatal(err)
		}
	}
	b := Buttons{}
	b.FunctionKeys[1] = true
	feed(b)
	feed(Buttons{})
	if c.Rendering.ViewControls.Mode != 7 {
		t.Fatal("source-eligible aircraft did not enter mast")
	}
	feed(Buttons{SceneDirections: [4]bool{true}})
	if err := c.StepRendering(1); err != nil {
		t.Fatal(err)
	}
	before := *c.Rendering.ViewControls
	feed(Buttons{AltQ: true, SceneDirections: [4]bool{true}})
	feed(Buttons{}) // Release the arrow while the modal still owns input.
	feed(Buttons{Escape: true})
	if c.Screen != ScreenScene {
		t.Fatal("modal did not cancel")
	}
	// The native loop can tick immediately after cancellation, before the
	// next ordinary input update. It must not replay the released direction.
	if err := c.StepRendering(1); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, *c.Rendering.ViewControls) {
		t.Fatal("released modal mast input changed the resumed view")
	}
}

func TestInitializedSceneClockIndependentOfDrawingAndFPS(t *testing.T) {
	makeView := func() *FrontEnd { c := renderingController(t); startRendering(t, c); return c }
	a, b := makeView(), makeView()
	for i := 0; i < 60; i++ {
		if e := a.AdvanceRendering(1.0 / 60); e != nil {
			t.Fatal(e)
		}
		a.Frame(false)
		a.Frame(false)
	}
	for i := 0; i < 30; i++ {
		if e := b.AdvanceRendering(1.0 / 30); e != nil {
			t.Fatal(e)
		}
		b.Frame(false)
	}
	if a.Rendering.Visual.Phase != 12 || b.Rendering.Visual.Phase != 12 {
		t.Fatal("frame dependent visual phase", a.Rendering.Visual.Phase, b.Rendering.Visual.Phase)
	}
	ea, eb := a.Rendering.Scene.Environment, b.Rendering.Scene.Environment
	if !reflect.DeepEqual(ea, eb) || !reflect.DeepEqual(a.Rendering.Initialized, b.Rendering.Initialized) {
		t.Fatal("drawing/FPS mutated initialization/weather")
	}
	camera := a.Rendering.Scene.Camera
	a.Handle(Actions{Quit: true})
	a.AdvanceRendering(2)
	a.Handle(Actions{Cancel: true})
	if a.Rendering.Scene.Camera != camera || a.Rendering.Visual.Phase != 12 {
		t.Fatal("visual/view tick behind modal")
	}
}

func TestCameraInputLatchEdgesAndModifiers(t *testing.T) {
	var w windowInput
	v := officeViewport(960, 720, false)
	b := Buttons{FunctionKeys: [9]bool{false, false, false, false, true}, ViewShift: true}
	a := w.ReadViewport(b, v)
	if a.ViewKey != 5 || !a.ViewShift {
		t.Fatal("ShiftF5 mapping")
	}
	if a = w.ReadViewport(b, v); a.ViewKey != 0 {
		t.Fatal("held F5 repeated")
	}
	w.ReadViewport(Buttons{}, v)
	a = w.ReadViewport(Buttons{PageUp: true}, v)
	if a.ViewCycle != 1 {
		t.Fatal("PageUp mapping")
	}
	a = w.ReadViewport(Buttons{PageUp: true}, v)
	if a.ViewCycle != 0 {
		t.Fatal("held PageUp repeated")
	}

}
