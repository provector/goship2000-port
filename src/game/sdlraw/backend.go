// Package sdlraw is shared by SETUP and the game. It initializes JOYSTICK/EVENTS,
// never VIDEO, audio, windows, renderers or any original game assets.
package sdlraw

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ebitengine/purego"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/internal/sdlruntime"

	"runtime"
	"unsafe"
)

type guid struct{ Lo, Hi uint64 }
type device struct {
	ptr  uintptr
	info controllers.Device
}
type Backend struct {
	eventState                func(int32) int32
	lib                       uintptr
	devices                   map[int32]device
	init                      func(uint32) int32
	quit                      func(uint32)
	hint                      func(string, string) int32
	update                    func()
	count                     func() int32
	open                      func(int32) uintptr
	close                     func(uintptr)
	instance                  func(int32) int32
	name, path                func(int32) uintptr
	serial                    func(uintptr) uintptr
	vendor, product, revision func(int32) uint16
	axes, buttons, hats       func(uintptr) int32
	axis                      func(uintptr, int32) int16
	button, hat               func(uintptr, int32) uint8
	getError                  func() uintptr
	wasInit                   func(uint32) uint32
	attached                  func(uintptr) int32
	getGUID                   func(int32) guid
}

func cstr(p uintptr) string {
	if p == 0 {
		return ""
	}
	b := make([]byte, 0, 64)
	for i := uintptr(0); i < 4096; i++ {
		c := *(*byte)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}
func Open() (_ *Backend, err error) {
	b := &Backend{devices: map[int32]device{}}
	b.lib, err = sdlruntime.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("SDL2 API unavailable: %v", v)
		}
		if err != nil {
			err = errors.Join(err, sdlruntime.Close(b.lib))
		}
	}()
	reg := func(fn any, name string) {
		p, e := sdlruntime.Symbol(b.lib, name)
		if e != nil {
			panic(e)
		}
		purego.RegisterFunc(fn, p)
	}
	reg(&b.init, "SDL_InitSubSystem")
	reg(&b.quit, "SDL_QuitSubSystem")
	reg(&b.hint, "SDL_SetHint")
	reg(&b.update, "SDL_JoystickUpdate")
	reg(&b.eventState, "SDL_JoystickEventState")
	reg(&b.count, "SDL_NumJoysticks")
	reg(&b.open, "SDL_JoystickOpen")
	reg(&b.close, "SDL_JoystickClose")
	reg(&b.instance, "SDL_JoystickGetDeviceInstanceID")
	reg(&b.name, "SDL_JoystickNameForIndex")
	reg(&b.path, "SDL_JoystickPathForIndex")
	reg(&b.serial, "SDL_JoystickGetSerial")
	reg(&b.vendor, "SDL_JoystickGetDeviceVendor")
	reg(&b.product, "SDL_JoystickGetDeviceProduct")
	reg(&b.revision, "SDL_JoystickGetDeviceProductVersion")
	reg(&b.axes, "SDL_JoystickNumAxes")
	reg(&b.buttons, "SDL_JoystickNumButtons")
	reg(&b.hats, "SDL_JoystickNumHats")
	reg(&b.axis, "SDL_JoystickGetAxis")
	reg(&b.button, "SDL_JoystickGetButton")
	reg(&b.hat, "SDL_JoystickGetHat")
	reg(&b.attached, "SDL_JoystickGetAttached")
	reg(&b.getError, "SDL_GetError")
	reg(&b.wasInit, "SDL_WasInit")
	b.getGUID = bindGUID(b.lib)
	b.hint("SDL_JOYSTICK_ALLOW_BACKGROUND_EVENTS", "1")
	if b.init(0x200) != 0 {
		return nil, fmt.Errorf("SDL joystick initialization: %s", cstr(b.getError()))
	}
	b.eventState(0) // Poll raw state; do not accumulate unused joystick events.
	return b, nil
}
func (b *Backend) Poll() (controllers.Snapshot, error) {
	if b.lib == 0 {
		return controllers.Snapshot{}, fmt.Errorf("controller backend closed")
	}
	b.update()
	s := controllers.Snapshot{States: map[int32]controllers.State{}, Subsystems: b.wasInit(0)}
	n := b.count()
	if n < 0 || n > 64 {
		return s, fmt.Errorf("controller count out of bounds: %d", n)
	}
	seen := map[int32]bool{}
	for i := int32(0); i < n; i++ {
		id := b.instance(i)
		seen[id] = true
		d, ok := b.devices[id]
		if !ok {
			ptr := b.open(i)
			if ptr == 0 {
				s.Error = fmt.Sprintf("Cannot open %s: %s", cstr(b.name(i)), cstr(b.getError()))
				continue
			}
			g := b.getGUID(i)
			raw := make([]byte, 16)
			binary.LittleEndian.PutUint64(raw, g.Lo)
			binary.LittleEndian.PutUint64(raw[8:], g.Hi)
			d = device{ptr: ptr, info: controllers.Device{Instance: id, Backend: "sdl2-raw-v1/" + runtime.GOOS, GUID: hex.EncodeToString(raw), Name: cstr(b.name(i)), Path: cstr(b.path(i)), Serial: cstr(b.serial(ptr)), Vendor: b.vendor(i), Product: b.product(i), Revision: b.revision(i), Axes: int(b.axes(ptr)), Buttons: int(b.buttons(ptr)), Hats: int(b.hats(ptr))}}
			if d.info.Axes < 0 || d.info.Axes > 128 || d.info.Buttons < 0 || d.info.Buttons > 512 || d.info.Hats < 0 || d.info.Hats > 16 {
				b.close(ptr)
				s.Error = "Controller capabilities exceed supported bounds"
				continue
			}
			b.devices[id] = d
		}
		if b.attached(d.ptr) == 0 {
			b.close(d.ptr)
			delete(b.devices, id)
			continue
		}
		s.Devices = append(s.Devices, d.info)
		v := controllers.State{Axes: make([]float64, d.info.Axes), Buttons: make([]bool, d.info.Buttons), Hats: make([]uint8, d.info.Hats)}
		for j := range v.Axes {
			x := b.axis(d.ptr, int32(j))
			v.Axes[j] = float64(x) / 32767
			if x < 0 {
				v.Axes[j] = float64(x) / 32768
			}
		}
		for j := range v.Buttons {
			v.Buttons[j] = b.button(d.ptr, int32(j)) != 0
		}
		for j := range v.Hats {
			v.Hats[j] = b.hat(d.ptr, int32(j))
		}
		s.States[id] = v
	}
	for id, d := range b.devices {
		if !seen[id] {
			b.close(d.ptr)
			delete(b.devices, id)
		}
	}
	return s, nil
}
func (b *Backend) Close() {
	if b.lib == 0 {
		return
	}
	for _, d := range b.devices {
		b.close(d.ptr)
	}
	b.devices = nil
	b.quit(0x200)
	// All polling and device handles belong to the caller on this thread.
	// Release our OS loader reference only after quitting the subsystem.
	_ = sdlruntime.Close(b.lib)
	*b = Backend{}
}
