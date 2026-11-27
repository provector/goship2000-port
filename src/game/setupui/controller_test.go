package setupui

import (
	"github.com/gdamore/tcell/v2"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func controllerUIForController(t *testing.T) *UI {
	t.Helper()
	d, _, e := configuration.Initialize(filepath.Join(t.TempDir(), configuration.Filename))
	if e != nil {
		t.Fatal(e)
	}
	u := New(d)
	device := controllers.Device{Instance: 0, Backend: "test/raw", GUID: "id", Name: "Test HOTAS", Axes: 4, Buttons: 16, Hats: 1}
	u.open("controls")
	u.applyControllerSnapshot(controllers.Snapshot{Devices: []controllers.Device{device}, States: map[int32]controllers.State{0: {Axes: []float64{0, 0, .8, 0}, Buttons: make([]bool, 16), Hats: []uint8{0}}}})
	u.selectDevice(device, "")
	// Retained Q2 fixture: explicit legacy polarity; Q31 tests cover new defaults.
	p := u.controllers.session.Profiles[u.controllers.session.Selected]
	p.CollectiveMode = "absolute" // retained Q2 explicit mode, not the new unconfigured default
	p.AxisConvention = ""
	p.CollectiveSource = ""
	u.controllers.session.Profiles[p.ID] = p
	u.controllers.session.Version = 2
	return u
}
func TestBasicDraftWizardSkipClearCancelAndRearm(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	u.openControllerEditor(false)
	old := controllers.Binding{Kind: "axis", Axis: 3}
	c.draft.Bindings["flight.pitch"] = old
	c.session.Profiles[c.session.Selected] = *c.draft
	u.finishControllerEditor(true)
	u.openControllerEditor(false)
	c.wizard = true
	c.step = 0
	u.beginCapture("flight.pitch")
	esc := tcell.NewEventKey(tcell.KeyEscape, 0, 0)
	u.App.GetInputCapture()(esc)
	if c.step != 1 || c.draft.Bindings["flight.pitch"].Axis != 3 {
		t.Fatal("wizard skip lost old")
	}
	c.keyUntil = time.Time{}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0))
	if c.step != 2 {
		t.Fatal("Space did not advance")
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, ' ', 0))
	if c.step != 2 {
		t.Fatal("held Space cleared several rows")
	}
	c.wizard = false
	u.App.GetInputCapture()(esc)
	if c.capture != nil {
		t.Fatal("manual cancel")
	}
	c.draft.Bindings["flight.pitch"] = controllers.Binding{Kind: "axis", Axis: 1}
	u.App.GetInputCapture()(esc)
	if c.draft != nil || c.session.Profiles[c.session.Selected].Bindings["flight.pitch"].Axis != 3 {
		t.Fatal("Basic Esc committed draft")
	}
	u.openControllerEditor(false)
	c.wizard = true
	c.step = 0
	u.beginCapture("flight.pitch")
	s := c.snapshot
	s.States[0].Axes[0] = 1
	u.applyControllerSnapshot(s)
	if c.capture == nil || c.step != 1 {
		t.Fatal("wizard missing return gate")
	}
	u.applyControllerSnapshot(s)
	if c.step != 1 || c.capture == nil {
		t.Fatal("held axis bound next step")
	}
	s.States[0].Axes[0] = 0
	u.applyControllerSnapshot(s)
	if c.rearm != nil || c.capture == nil {
		t.Fatal("axis failed to rearm")
	}
	u.applyControllerSnapshot(controllers.Snapshot{})
	if c.connected || c.capture != nil || c.wizard {
		t.Fatal("unplug did not cancel")
	}
}
func TestWholePairAndExplicitCollisionDoNotReplaceEarly(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	u.openControllerEditor(false)
	id := "flight.roll"
	c.draft.Bindings[id] = controllers.Binding{Kind: "axis", Axis: 2}
	u.beginCapture(id)
	s := c.snapshot
	s.States[0].Buttons[0] = true
	u.applyControllerSnapshot(s)
	if c.draft.Bindings[id].Axis != 2 || c.capture.Negative == nil {
		t.Fatal("partial pair changed prior")
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if c.draft.Bindings[id].Axis != 2 {
		t.Fatal("cancel destroyed prior")
	}
	c.draft.Bindings["weapon.fire"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 1}}
	u.beginCapture("menu.selector1")
	s.States[0].Buttons[0] = false
	s.States[0].Buttons[1] = true
	u.applyControllerSnapshot(s)
	if c.pending != nil {
		t.Fatal("different contexts erroneously collided")
	}
	u.beginCapture("weapon.chaff")
	s.States[0].Buttons[1] = false
	u.applyControllerSnapshot(s)
	s.States[0].Buttons[1] = true
	u.applyControllerSnapshot(s)
	if c.pending == nil || c.draft.Bindings["weapon.chaff"].Kind != "" {
		t.Fatal("collision silently committed")
	}
	u.App.GetInputCapture()(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	if b, _ := controllers.EffectiveBinding(*c.draft, "weapon.fire"); b.Kind == "" {
		t.Fatal("collision cancel destroyed old binding")
	}
}
func TestPreviewCompactFramesButtonsAndInvert(t *testing.T) {
	u := controllerUIForController(t)
	u.Doc.Config.Runtime.SwapYawRollAxis = false // Retained unswapped Q2 preview; default On is covered by Q3.
	c := u.controllers
	u.openControllerEditor(false)
	c.draft.Bindings["flight.roll"] = controllers.Binding{Kind: "axis", Axis: 0}
	c.draft.Bindings["flight.yaw"] = controllers.Binding{Kind: "axis", Axis: 1, Invert: true}
	s := c.snapshot
	s.States[0].Axes[0] = 1
	s.States[0].Axes[1] = 1
	s.States[0].Buttons[0] = true
	s.States[0].Buttons[11] = true
	u.applyControllerSnapshot(s)
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.Init()
	defer screen.Fini()
	for _, size := range [][2]int{{76, 24}, {80, 25}} {
		screen.SetSize(size[0], size[1])
		u.App.GetBeforeDrawFunc()(screen)
		u.Main.SetRect(0, 0, size[0], size[1])
		u.Main.Draw(screen)
		for _, point := range [][2]int{{14, 8}, {25, 8}} {
			ch, _, _, _ := screen.GetContent(point[0], point[1])
			if ch != '█' {
				t.Fatal("wrong mapped cross", size, point, ch)
			}
		}
		for _, point := range [][2]int{{53, 6}, {68, 8}} {
			_, _, style, _ := screen.GetContent(point[0], point[1])
			_, bg, _ := style.Decompose()
			if bg.Hex() != yellow.Hex() {
				t.Fatal("simultaneous button not filled", point)
			}
		}
		ch, _, _, _ := screen.GetContent(0, size[1]-1)
		if ch != '↑' {
			t.Fatal("footer overlapped")
		}
	}
	s.States[0].Buttons[0] = false
	s.States[0].Buttons[11] = false
	u.applyControllerSnapshot(s)
	if c.snapshot.States[0].Buttons[0] {
		t.Fatal("release stale")
	}
}
func TestMainSaveReopenNoneDiscardAndCompanionFailure(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	initial, err := os.ReadFile(c.doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	u.openControllerEditor(false)
	c.draft.Bindings["flight.yaw"] = controllers.Binding{Kind: "axis", Axis: 1, Invert: true}
	u.finishControllerEditor(true)
	if disk, e := os.ReadFile(c.doc.Path); e != nil || string(disk) != string(initial) {
		t.Fatal("Basic Save wrote disk")
	}
	if e := u.saveSession(); e != nil {
		t.Fatal(e)
	}
	d, e := controllers.Load(c.doc.Path)
	if e != nil || !d.Config.Profiles[d.Config.Selected].Bindings["flight.yaw"].Invert {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(c.doc.Path)
	c.session.Selected = ""
	after, _ := os.ReadFile(c.doc.Path)
	if string(before) != string(after) {
		t.Fatal("session selection wrote early")
	}
	u.Doc.Config.WindowWidth = -1
	if e = u.saveSession(); e == nil {
		t.Fatal("invalid companion saved")
	}
	after, _ = os.ReadFile(c.doc.Path)
	if string(before) != string(after) {
		t.Fatal("controls not rolled back on companion failure")
	}
}
func TestIdenticalDeviceLossRequiresChoiceAndShortProfileID(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	s := c.snapshot
	d := s.Devices[0]
	other := d
	other.Instance = 9
	s.Devices = append(s.Devices, other)
	s.States[9] = s.States[0]
	u.applyControllerSnapshot(s)
	u.selectDevice(d, c.session.Selected)
	u.openControllerEditor(false)
	u.beginCapture("flight.pitch")
	s.Devices = []controllers.Device{other}
	delete(s.States, 0)
	u.applyControllerSnapshot(s)
	if c.connected || c.capture != nil || !c.chooseAfterLoss {
		t.Fatal("identical survivor silently took ownership")
	}
	u.selectDevice(other, c.session.Selected)
	if !c.connected {
		t.Fatal("explicit choice did not reconnect")
	}
	p := c.session.Profiles[c.session.Selected]
	p.ID = "x"
	u.profileChoices(other, []controllers.Profile{p}) // no minimum opaque ID length is assumed
}
func TestAdvancedAcceptAndDiscardShareBasicSelector(t *testing.T) {
	u := controllerUIForController(t)
	c := u.controllers
	u.openControllerEditor(true)
	c.draft.Bindings["menu.selector2"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 15}}
	u.finishControllerEditor(true)
	row, _ := u.Table.GetSelection()
	if row != 3 {
		t.Fatal("Advanced parent focus", row)
	}
	u.openControllerEditor(false)
	if c.draft.Bindings["menu.selector2"].Control.Index != 15 {
		t.Fatal("Advanced/Basic disagree")
	}
	u.finishControllerEditor(false)
	u.openControllerEditor(true)
	delete(c.draft.Bindings, "menu.selector2")
	u.finishControllerEditor(false)
	if c.session.Profiles[c.session.Selected].Bindings["menu.selector2"].Control.Index != 15 {
		t.Fatal("discard overwrote accepted binding")
	}
}
