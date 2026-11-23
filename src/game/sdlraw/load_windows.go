//go:build windows

package sdlraw

import (
	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
	"path/filepath"
)

func libraryNames(dir string) []string { return []string{filepath.Join(dir, "SDL2.dll")} }
func load(name string) (uintptr, error) {
	h, e := windows.LoadLibraryEx(name, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	return uintptr(h), e
}
func symbol(lib uintptr, name string) (uintptr, error) {
	return windows.GetProcAddress(windows.Handle(lib), name)
}

// Win64 returns this16-byte C aggregate through a caller-supplied first pointer.
// Linux/ARM64 use the native struct ABI in their separate adapter.
func bindGUID(lib uintptr) func(int32) guid {
	var f func(*guid, int32) uintptr
	p, e := symbol(lib, "SDL_JoystickGetDeviceGUID")
	if e != nil {
		panic(e)
	}
	purego.RegisterFunc(&f, p)
	return func(i int32) guid { var g guid; f(&g, i); return g }
}

func unload(lib uintptr) error { return windows.FreeLibrary(windows.Handle(lib)) }
