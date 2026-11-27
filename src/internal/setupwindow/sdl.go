// Package setupwindow presents a tview application in an SDL2 window.
package setupwindow

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"gunship2000-assets/internal/sdlruntime"
)

// SDL2 ABI shared by the three supported little-endian 64-bit targets.
const rgba32 = 0x16762004 // SDL_PIXELFORMAT_ABGR8888
type api struct {
	init                func(uint32) int32
	quit                func(uint32)
	hint                func(string, string) int32
	lastError           func() string
	createWindow        func(string, int32, int32, int32, int32, uint32) uintptr
	destroyWindow       func(uintptr)
	minSize             func(uintptr, int32, int32)
	size                func(uintptr, *int32, *int32)
	flags               func(uintptr) uint32
	icon                func(uintptr, uintptr)
	surface             func(*uint8, int32, int32, int32, int32, uint32) uintptr
	freeSurface         func(uintptr)
	createRenderer      func(uintptr, int32, uint32) uintptr
	destroyRenderer     func(uintptr)
	createTexture       func(uintptr, uint32, int32, int32, int32) uintptr
	updateTexture       func(uintptr, uintptr, *byte, int32) int32
	destroyTexture      func(uintptr)
	outputSize          func(uintptr, *int32, *int32) int32
	clear               func(uintptr) int32
	copyTexture         func(uintptr, uintptr, uintptr, *[4]int32) int32
	present             func(uintptr)
	wait                func(*event, int32) int32
	poll                func(*event) int32
	startText, stopText func()
	mods                func() int32
	clipboard           func() *byte
	strlen              func(*byte) uintptr
	free                func(unsafe.Pointer)
}

func (a *api) bind(lib uintptr) error {
	for _, f := range []struct {
		name string
		dst  any
	}{
		{"InitSubSystem", &a.init}, {"QuitSubSystem", &a.quit}, {"SetHint", &a.hint}, {"GetError", &a.lastError},
		{"CreateWindow", &a.createWindow}, {"DestroyWindow", &a.destroyWindow}, {"SetWindowMinimumSize", &a.minSize}, {"GetWindowSize", &a.size}, {"GetWindowFlags", &a.flags}, {"SetWindowIcon", &a.icon}, {"CreateRGBSurfaceWithFormatFrom", &a.surface}, {"FreeSurface", &a.freeSurface},
		{"CreateRenderer", &a.createRenderer}, {"DestroyRenderer", &a.destroyRenderer}, {"CreateTexture", &a.createTexture}, {"UpdateTexture", &a.updateTexture}, {"DestroyTexture", &a.destroyTexture}, {"GetRendererOutputSize", &a.outputSize}, {"RenderClear", &a.clear}, {"RenderCopy", &a.copyTexture}, {"RenderPresent", &a.present},
		{"WaitEventTimeout", &a.wait}, {"PollEvent", &a.poll}, {"StartTextInput", &a.startText}, {"StopTextInput", &a.stopText}, {"GetModState", &a.mods}, {"GetClipboardText", &a.clipboard}, {"strlen", &a.strlen}, {"free", &a.free},
	} {
		p, e := sdlruntime.Symbol(lib, "SDL_"+f.name)
		if e != nil {
			return fmt.Errorf("SDL_%s: %w", f.name, e)
		}
		purego.RegisterFunc(f.dst, p)
	}
	return nil
}
func (a *api) setIcon(w uintptr, data []byte) {
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	b := im.Bounds()
	n := image.NewNRGBA(b)
	draw.Draw(n, b, im, b.Min, draw.Src)
	s := a.surface(&n.Pix[0], int32(b.Dx()), int32(b.Dy()), 32, int32(n.Stride), rgba32)
	if s != 0 {
		a.icon(w, s)
		a.freeSurface(s)
	}
	runtime.KeepAlive(n)
}
