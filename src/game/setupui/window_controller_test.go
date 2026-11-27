package setupui

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/sdlraw"
	"gunship2000-assets/internal/sdlruntime"
)

// A virtual SDL joystick exercises the native driver and window-to-widget
// delivery. All device operations run on the same owner as real SETUP.
type testJoystick struct {
	controllers.Backend
	commands chan func()
	cleanup  func()
}

func (v *testJoystick) Poll() (controllers.Snapshot, error) {
	select {
	case f := <-v.commands:
		f()
	default:
	}
	return v.Backend.Poll()
}
func (v *testJoystick) Close() { v.cleanup(); v.Backend.Close() }
func TestWindowRawCaptureFocusAndReconnect(t *testing.T) {
	t.Setenv("SDL_VIDEODRIVER", "dummy")
	d, _, err := configuration.Initialize(filepath.Join(t.TempDir(), configuration.Filename))
	if err != nil {
		t.Fatal(err)
	}
	u := New(d)
	lib, err := sdlruntime.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer sdlruntime.Close(lib)
	var attach func(int32, int32, int32, int32) int32
	var detach func(int32) int32
	var open func(int32) uintptr
	var closeJoy func(uintptr)
	var axis func(uintptr, int32, int16) int32
	var button, hat func(uintptr, int32, uint8) int32
	var push func(*[7]uint64) int32
	var id func(uintptr) int32
	for _, x := range []struct {
		s string
		p any
	}{{"SDL_JoystickAttachVirtual", &attach}, {"SDL_JoystickDetachVirtual", &detach}, {"SDL_JoystickOpen", &open}, {"SDL_JoystickClose", &closeJoy}, {"SDL_JoystickSetVirtualAxis", &axis}, {"SDL_JoystickSetVirtualButton", &button}, {"SDL_JoystickSetVirtualHat", &hat}, {"SDL_JoystickInstanceID", &id}, {"SDL_PushEvent", &push}} {
		p, e := sdlruntime.Symbol(lib, x.s)
		if e != nil {
			t.Fatal(e)
		}
		purego.RegisterFunc(x.p, p)
	}
	focus := func(gain bool) {
		var ev [7]uint64
		b := unsafe.Slice((*byte)(unsafe.Pointer(&ev)), 56)
		binary.LittleEndian.PutUint32(b, 0x200)
		b[12] = 13
		if gain {
			b[12] = 12
		}
		push(&ev)
	}
	commands := make(chan func(), 1)
	ready := make(chan struct{})
	var index, instance int32
	var ptr uintptr
	connect := func() { index = attach(1, 4, 8, 1); ptr = open(index); instance = id(ptr) }
	disconnect := func() {
		if ptr != 0 {
			closeJoy(ptr)
			ptr = 0
		}
		if index >= 0 {
			detach(index)
			index = -1
		}
	}
	u.ControllerBackend = func() (controllers.Backend, error) {
		b, e := sdlraw.Open()
		if e != nil {
			return nil, e
		}
		connect()
		if index < 0 || ptr == 0 {
			disconnect()
			b.Close()
			return nil, fmt.Errorf("virtual joystick unavailable")
		}
		close(ready)
		return &testJoystick{b, commands, disconnect}, nil
	}
	result := make(chan error, 1)
	go func() {
		select {
		case <-ready:
		case <-time.After(8 * time.Second):
			result <- fmt.Errorf("controller initialization timed out")
			u.App.Stop()
			return
		}
		// The dummy video driver has no desktop focus manager.
		focus(true)
		wait := func(label string, check func() bool) bool {
			end := time.Now().Add(4 * time.Second)
			for time.Now().Before(end) {
				ok := false
				u.App.QueueUpdate(func() { ok = check() })
				if ok {
					return true
				}
				time.Sleep(12 * time.Millisecond)
			}
			result <- fmt.Errorf("%s did not reach widget", label)
			u.App.Stop()
			return false
		}
		var device controllers.Device
		if !wait("enumeration", func() bool {
			for _, v := range u.controllers.snapshot.Devices {
				if v.Instance == instance {
					device = v
					return true
				}
			}
			return false
		}) {
			return
		}
		var profile string
		u.App.QueueUpdate(func() {
			u.open("controls")
			u.selectDevice(device, "")
			profile = u.controllers.session.Selected
			u.openControllerEditor(false)
			u.beginCapture("flight.pitch")
		})
		commands <- func() { axis(ptr, 2, -26000) }
		if !wait("axis", func() bool {
			return u.controllers.draft.Bindings["flight.pitch"].Kind == "axis" && u.controllers.draft.Bindings["flight.pitch"].Axis == 2
		}) {
			return
		}
		u.App.QueueUpdate(func() { u.beginCapture("menu.selector1") })
		commands <- func() { button(ptr, 5, 1) }
		if !wait("button", func() bool {
			b := u.controllers.draft.Bindings["menu.selector1"]
			return b.Control != nil && b.Control.Kind == "button" && b.Control.Index == 5
		}) {
			return
		}
		u.App.QueueUpdate(func() { u.beginCapture("menu.selector2") })
		focus(false)
		if !wait("focus cancellation", func() bool {
			return !u.popup && u.controllers.capture == nil && u.controllers.draft.Bindings["menu.selector2"].Kind == ""
		}) {
			return
		}
		focus(true)
		u.App.QueueUpdate(func() { u.beginCapture("menu.selector2") })
		commands <- func() { hat(ptr, 0, 2) }
		if !wait("hat", func() bool {
			b := u.controllers.draft.Bindings["menu.selector2"]
			return b.Control != nil && b.Control.Kind == "hat" && b.Control.Direction == 2
		}) {
			return
		}
		u.App.QueueUpdate(func() { u.beginCapture("flight.roll") })
		commands <- disconnect
		if !wait("unplug cancellation", func() bool { return !u.controllers.connected && u.controllers.capture == nil && !u.popup }) {
			return
		}
		commands <- connect
		if !wait("reconnect", func() bool { return u.controllers.connected && u.controllers.device.Instance != device.Instance }) {
			return
		}
		u.App.QueueUpdate(func() { u.finishControllerEditor(true); u.saveAndExit(false) })
		cfg, e := controllers.Load(d.Path)
		if e == nil && (!u.Saved || cfg.Config.Selected != profile || len(cfg.Config.Profiles[profile].Bindings) < 3) {
			e = fmt.Errorf("saved profile missing captured bindings")
		}
		result <- e
	}()
	if e := u.RunWindow(); e != nil {
		t.Fatal(e)
	}
	if e := <-result; e != nil {
		t.Fatal(e)
	}

	// Reopen the saved document using the game's read-only runtime consumer.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cfg, err := controllers.Load(d.Path)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sdlraw.Open()
	if err != nil {
		t.Fatal(err)
	}
	live := controllers.NewLive(cfg.Config, raw)
	defer live.Close()
	connect()
	defer disconnect()
	if v := live.Poll("flight", true); !v.Connected {
		t.Fatal("saved device unavailable", v.Status)
	}
	axis(ptr, 2, 26000)
	v := live.Poll("flight", true)
	if v.Axes[0] != 101 {
		t.Fatal("saved pitch binding ineffective in flight", v)
	}
	live.Poll("office", true)
	button(ptr, 5, 1)
	if !live.Poll("office", true).Down["menu.selector1"] {
		t.Fatal("saved selector ineffective")
	}
}
