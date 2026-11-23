package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func captureFixture() (*mouseCapture, mouseCaptureSample) {
	p := &mouseCapture{}
	s := mouseCaptureSample{Enabled: true, Focused: true, Client: true, AbsoluteX: 100, AbsoluteY: 90, AbsoluteInside: true, Viewport: Viewport{Width: 960, Height: 600}, Epoch: "office"}
	p.Read(s)
	return p, s
}

func TestCaptureConsumesAcquisitionAndBackendWarp(t *testing.T) {
	p, s := captureFixture()
	var w windowInput
	read := func() (mouseCaptureResult, Actions) {
		r := p.Read(s)
		a := w.ReadViewport(Buttons{LeftMouse: s.Left && !r.SuppressButtons, RightMouse: s.Right && !r.SuppressButtons, X: r.X, Y: r.Y, Inside: r.Inside, PointerRelative: r.Relative, PointerDX: r.DX, PointerDY: r.DY, PointerEpoch: r.Reset}, s.Viewport)
		return r, a
	}
	s.Left = true
	r, a := read()
	if !r.Acquire || !r.Captured || a.Selector1 || !a.PointerReset {
		t.Fatal("capture click activated a game control", r, a)
	}
	s.DeltaX, s.DeltaY = 3000, -3000
	r, a = read()
	if r.DX != 0 || r.DY != 0 || a.Selector1 {
		t.Fatal("backend recenter/held acquisition leaked", r, a)
	}
	s.Left, s.DeltaX, s.DeltaY = false, 0, 0
	read()
	s.Left = true
	_, a = read()
	if !a.Selector1 || !a.PointerPressed {
		t.Fatal("fresh click did not activate normally", a)
	}
}

func TestCaptureScalingFractionEdgesAndReleaseChord(t *testing.T) {
	p, s := captureFixture()
	s.Right = true
	p.Read(s)
	s.Right = false
	p.Read(s) // discard backend transition
	s.DeltaX, s.DeltaY = 1, 1
	for i := 0; i < 2; i++ {
		if r := p.Read(s); r.DX != 0 || r.DY != 0 {
			t.Fatal("fraction lost", r)
		}
	}
	if r := p.Read(s); r.DX != 1 || r.DY != 1 {
		t.Fatal("inverse viewport scale should retain fractional motion", r)
	}
	s.DeltaX, s.DeltaY = 9000, 9000
	for i := 0; i < 4; i++ {
		p.Read(s)
	}
	s.DeltaX, s.DeltaY = -3, -3
	if r := p.Read(s); r.X != 318 || r.Y != 189 || r.DX != -1 || r.DY != -1 {
		t.Fatal("free pointer accumulated outward overshoot", r)
	}
	s.CtrlF10, s.F10 = true, true
	if r := p.Read(s); !r.Release || r.Captured || !r.SuppressF10 {
		t.Fatal("release chord did not unlock", r)
	}
	// The backend's EnableCursor recenter is an absolute sample while released.
	// It must not become hidden menu movement or the next capture's game anchor.
	s.AbsoluteX, s.AbsoluteY = 160, 100
	if r := p.Read(s); r.X != 318 || r.Y != 189 || !r.Relative || r.DX != 0 || r.DY != 0 {
		t.Fatal("released backend recenter changed the logical pointer", r)
	}
	s.CtrlF10, s.Right = false, true
	if r := p.Read(s); !r.SuppressF10 || r.Acquire {
		t.Fatal("Ctrl release while F10 held reactivated chord", r)
	}
	s.F10, s.Right = false, false
	p.Read(s)
	s.Right = true
	if r := p.Read(s); !r.Acquire || !r.SuppressButtons {
		t.Fatal("subsequent fresh click did not recapture", r)
	}
	if p.x != 318 || p.y != 189 {
		t.Fatal("reacquisition adopted the host cursor position")
	}
}

func TestCaptureFocusResizeModalEpochsAndDisabledFallback(t *testing.T) {
	p, s := captureFixture()
	s.Left = true
	p.Read(s)
	s.Left = false
	p.Read(s)
	s.DeltaX, s.DeltaY = 6, 6
	r := p.Read(s)
	x, y := r.X, r.Y
	s.Viewport = Viewport{Width: 800, Height: 600}
	s.DeltaX, s.DeltaY = 9999, 9999
	r = p.Read(s)
	if !r.Reset || r.X != x || r.Y != y || r.DX != 0 || r.DY != 0 {
		t.Fatal("resize moved logical pointer", r)
	}
	s.Epoch = "quit"
	r = p.Read(s)
	if !r.Reset || !r.Captured || r.X != x || r.Y != y {
		t.Fatal("in-game modal released capture or moved state", r)
	}
	s.Focused, s.Left = false, true
	r = p.Read(s)
	if !r.Release || !r.SuppressButtons || r.Inside {
		t.Fatal("focus loss retained capture/input", r)
	}
	s.Focused = true
	if r = p.Read(s); r.Acquire || !r.SuppressButtons {
		t.Fatal("held focus-restoration press recaptured", r)
	}
	s.Left = false
	p.Read(s)
	s.Left = true
	if r = p.Read(s); !r.Acquire {
		t.Fatal("fresh focus-restored click should recapture", r)
	}
	s.Enabled = false
	if r = p.Read(s); !r.Release || r.Relative {
		t.Fatal("disabled policy did not return to absolute input", r)
	}
}

func TestCapturedRowsUseRawDeltaAfterFreePointerClamps(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	v.updateRowPointer(Actions{Inside: true, PointerRelative: true, PointerReset: true, X: 319, Y: 190})
	for i := 0; i < 4; i++ {
		v.updateRowPointer(Actions{Inside: true, PointerRelative: true, X: 319, Y: 190, PointerDY: 1000})
	}
	before := v.rowPointer.y
	v.updateRowPointer(Actions{Inside: true, PointerRelative: true, X: 319, Y: 190, PointerDY: -1})
	if v.rowPointer.y != before-1 {
		t.Fatal("row consumed clamped free cursor rather than direct delta")
	}
	v.updateRowPointer(Actions{Inside: true, PointerRelative: true, X: 319, Y: 190, MenuHome: true})
	selected := v.Hover
	v.updateRowPointer(Actions{Inside: true, PointerRelative: true, X: 319, Y: 190})
	if v.Hover != selected {
		t.Fatal("stationary captured pointer stole keyboard selection")
	}
}

func TestCaptureThroughWindowLatchAndMissionController(t *testing.T) {
	c := sourceFrontEnd(t, true)
	if err := c.openMission(); err != nil {
		t.Fatal(err)
	}
	before := c.Store.State().Revision
	p, s := captureFixture()
	s.Epoch, s.AbsoluteY = string(c.Screen), 82
	var w windowInput
	step := func() Actions {
		t.Helper()
		r := p.Read(s)
		a := w.ReadViewport(Buttons{LeftMouse: s.Left && !r.SuppressButtons, RightMouse: s.Right && !r.SuppressButtons, X: r.X, Y: r.Y, Inside: r.Inside, PointerRelative: r.Relative, PointerDX: r.DX, PointerDY: r.DY, PointerEpoch: r.Reset}, s.Viewport)
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	step()
	s.Left = true
	if a := step(); a.Selector1 || c.Screen != ScreenMission || c.Store.State().Revision != before {
		t.Fatal("acquisition click crossed actual controller boundary")
	}
	s.Left = false
	step()
	s.DeltaX, s.DeltaY = 0, 9000
	for i := 0; i < 4; i++ {
		step()
	}
	if c.Hit(c.Office.PointerX, c.Office.PointerY) != -1 {
		t.Fatal("chooser bottom margin became selectable")
	}
	s.DeltaY, s.Left = 0, true
	step()
	if c.Screen != ScreenMission || c.Store.State().Revision != before {
		t.Fatal("margin click committed previous highlighted choice")
	}
	s.Left, s.DeltaY = false, -3
	step()
	if c.Office.PointerY != 101 {
		t.Fatal("raw captured reversal was lost after free cursor clamped", c.Office.PointerY)
	}
	oldX, oldY := c.Office.PointerX, c.Office.PointerY
	s.Focused = false
	step()
	s.Focused, s.AbsoluteX, s.AbsoluteY, s.DeltaY = true, 160, 100, 9000
	step()
	s.Left = true
	step()
	if c.Office.PointerX != oldX || c.Office.PointerY != oldY {
		t.Fatal("focus/recapture lost the chooser's confined anchor", c.Office.PointerX, c.Office.PointerY, oldX, oldY)
	}
}
