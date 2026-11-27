package sdlraw

import (
	"github.com/ebitengine/purego"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/internal/sdlruntime"
	"runtime"
	"testing"
)

func TestSDLRawVirtualLifecycleWithoutVideo(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b, e := Open()
	if e != nil {
		t.Skip("optional native SDL runtime unavailable: " + e.Error())
	}
	defer b.Close()
	var attach func(int32, int32, int32, int32) int32
	var detach func(int32) int32
	var setAxis func(uintptr, int32, int16) int32
	var setButton func(uintptr, int32, uint8) int32
	var setHat func(uintptr, int32, uint8) int32
	reg := func(f any, n string) {
		p, e := sdlruntime.Symbol(b.lib, n)
		if e != nil {
			t.Fatal(e)
		}
		purego.RegisterFunc(f, p)
	}
	reg(&attach, "SDL_JoystickAttachVirtual")
	reg(&detach, "SDL_JoystickDetachVirtual")
	reg(&setAxis, "SDL_JoystickSetVirtualAxis")
	reg(&setButton, "SDL_JoystickSetVirtualButton")
	reg(&setHat, "SDL_JoystickSetVirtualHat")
	index := attach(0, 3, 16, 1)
	if index < 0 {
		t.Fatal(cstr(b.getError()))
	}
	attached := true
	defer func() {
		if attached {
			detach(index)
		}
	}()
	id := b.instance(index)
	s, e := b.Poll()
	if e != nil {
		t.Fatal(e)
	}
	if s.Subsystems&0x20 != 0 {
		t.Fatal("joystick backend initialized video")
	}
	d, ok := b.devices[id]
	if !ok || d.info.Axes != 3 || d.info.Buttons != 16 || d.info.Hats != 1 {
		t.Fatal("raw capabilities lost", d)
	}
	if setAxis(d.ptr, 0, 32767) != 0 || setButton(d.ptr, 0, 1) != 0 || setButton(d.ptr, 15, 1) != 0 || setHat(d.ptr, 0, 3) != 0 {
		t.Fatal(cstr(b.getError()))
	}
	s, e = b.Poll()
	if e != nil {
		t.Fatal(e)
	}
	state := s.States[id]
	if state.Axes[0] != 1 || !state.Buttons[0] || !state.Buttons[15] || state.Hats[0] != 3 {
		t.Fatal("polled raw input missing", state)
	}
	for _, value := range []uint8{0, 1, 2, 3, 4, 6, 8, 9, 12, 0} {
		if setHat(d.ptr, 0, value) != 0 {
			t.Fatal(cstr(b.getError()))
		}
		snap, err := b.Poll()
		if err != nil || snap.States[id].Hats[0] != value {
			t.Fatal("raw hat direction lost", value, err)
		}
	}
	// Actual SDL raw delivery into endpoint capture; this is virtual hardware.
	setAxis(d.ptr, 0, 0)
	snap, _ := b.Poll()
	negative := controllers.NewCapture("flight.roll_left", snap.States[id])
	setAxis(d.ptr, 0, -32768)
	snap, _ = b.Poll()
	left, ok := negative.Update(snap.States[id])
	if !ok || left.Control.Sign != -1 {
		t.Fatal("native negative endpoint")
	}
	positive := controllers.NewCapture("flight.roll_right", snap.States[id])
	setAxis(d.ptr, 0, 32767)
	snap, _ = b.Poll()
	if _, done := positive.Update(snap.States[id]); done {
		t.Fatal("native endpoint crossed without neutral")
	}
	setAxis(d.ptr, 0, 0)
	snap, _ = b.Poll()
	positive.Update(snap.States[id])
	setAxis(d.ptr, 0, 32767)
	snap, _ = b.Poll()
	right, ok := positive.Update(snap.States[id])
	if !ok || right.Control.Sign != 1 {
		t.Fatal("native positive endpoint")
	}
	if detach(index) != 0 {
		t.Fatal(cstr(b.getError()))
	}
	attached = false
	s, e = b.Poll()
	if e != nil {
		t.Fatal(e)
	}
	if _, exists := s.States[id]; exists {
		t.Fatal("detached device retained held state")
	}
}

func TestSDLVirtualFeedsReadOnlyGameSession(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b, e := Open()
	if e != nil {
		t.Skip("optional native SDL unavailable: " + e.Error())
	}
	var attach func(int32, int32, int32, int32) int32
	var detach func(int32) int32
	var setAxis func(uintptr, int32, int16) int32
	var setButton func(uintptr, int32, uint8) int32
	reg := func(f any, n string) {
		p, e := sdlruntime.Symbol(b.lib, n)
		if e != nil {
			t.Fatal(e)
		}
		purego.RegisterFunc(f, p)
	}
	reg(&attach, "SDL_JoystickAttachVirtual")
	reg(&detach, "SDL_JoystickDetachVirtual")
	reg(&setAxis, "SDL_JoystickSetVirtualAxis")
	reg(&setButton, "SDL_JoystickSetVirtualButton")
	index := attach(0, 4, 8, 1)
	if index < 0 {
		t.Fatal(cstr(b.getError()))
	}
	attached := true
	defer func() {
		if attached {
			detach(index)
		}
		b.Close()
	}()
	snap, e := b.Poll()
	if e != nil {
		t.Fatal(e)
	}
	id := b.instance(index)
	dev := b.devices[id]
	p := controllers.Profile{ID: "raw", Device: dev.info, CollectiveMode: "absolute", Bindings: map[string]controllers.Binding{"flight.roll": {Kind: "axis", Axis: 0}, "flight.pitch": {Kind: "axis", Axis: 1, Invert: true}, "flight.collective": {Kind: "axis", Axis: 2}, "menu.selector1": {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 0}}}}
	live := controllers.NewLive(controllers.Config{Version: 2, Selected: "raw", Profiles: map[string]controllers.Profile{"raw": p}}, b)
	live.Poll("office", true)
	setAxis(dev.ptr, 0, 16384)
	setAxis(dev.ptr, 1, -32768)
	setAxis(dev.ptr, 2, 32767)
	setButton(dev.ptr, 0, 1)
	got := live.Poll("office", true)
	if got.Axes[1] != 64 || got.Axes[0] != 127 || got.Lever != 65535 || !got.Down["menu.selector1"] {
		t.Fatal(got, snap)
	}
	got = live.Poll("flight", true)
	if got.Down["menu.selector1"] || got.Axes[0] != 0 {
		t.Fatal("context rearm", got)
	}
	detach(index)
	attached = false
	got = live.Poll("flight", true)
	if got.Connected || got.Down["menu.selector1"] {
		t.Fatal("disconnect", got)
	}
}

func TestTwoBackendReferencesCloseIndependently(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	first, err := Open()
	if err != nil {
		t.Skip("optional SDL runtime: ", err)
	}
	defer first.Close()
	second, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	first.Close()
	if _, err := first.Poll(); err == nil {
		t.Fatal("closed loader reference admitted polling")
	}
	if _, err := second.Poll(); err != nil {
		t.Fatal(err)
	}
}
