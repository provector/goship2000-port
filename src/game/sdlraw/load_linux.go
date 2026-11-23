//go:build linux

package sdlraw

import (
	"github.com/ebitengine/purego"
	"path/filepath"
)

func libraryNames(dir string) []string {
	return []string{filepath.Join(dir, "lib", "libSDL2-2.0.so.0"), "libSDL2-2.0.so.0"}
}
func load(name string) (uintptr, error) {
	return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
}
func symbol(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }
func bindGUID(lib uintptr) func(int32) guid {
	var f func(int32) guid
	p, e := symbol(lib, "SDL_JoystickGetDeviceGUID")
	if e != nil {
		panic(e)
	}
	purego.RegisterFunc(&f, p)
	return f
}

func unload(lib uintptr) error { return purego.Dlclose(lib) }
