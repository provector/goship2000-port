//go:build darwin || linux

package sdlruntime

import "github.com/ebitengine/purego"

func load(name string) (uintptr, error) {
	return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
}
func Symbol(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }
func Close(lib uintptr) error                          { return purego.Dlclose(lib) }
