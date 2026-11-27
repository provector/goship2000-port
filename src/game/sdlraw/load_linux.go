//go:build linux

package sdlraw

import (
	"github.com/ebitengine/purego"
	"gunship2000-assets/internal/sdlruntime"
)

func bindGUID(lib uintptr) func(int32) guid {
	var f func(int32) guid
	p, e := sdlruntime.Symbol(lib, "SDL_JoystickGetDeviceGUID")
	if e != nil {
		panic(e)
	}
	purego.RegisterFunc(&f, p)
	return f
}
