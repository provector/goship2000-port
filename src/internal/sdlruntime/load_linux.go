//go:build linux

package sdlruntime

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
func Symbol(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }
func Close(lib uintptr) error                          { return purego.Dlclose(lib) }
