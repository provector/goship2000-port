//go:build windows

package sdlraw

import (
	"github.com/ebitengine/purego"

	"gunship2000-assets/internal/sdlruntime"
)

func bindGUID(lib uintptr) func(int32) guid {
	var f func(*guid, int32) uintptr
	p, e := sdlruntime.Symbol(lib, "SDL_JoystickGetDeviceGUID")
	if e != nil {
		panic(e)
	}
	purego.RegisterFunc(&f, p)
	return func(i int32) guid { var g guid; f(&g, i); return g }
}
