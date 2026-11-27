//go:build windows

package sdlruntime

import (
	"golang.org/x/sys/windows"
	"path/filepath"
)

func libraryNames(dir string) []string { return []string{filepath.Join(dir, "SDL2.dll")} }
func load(name string) (uintptr, error) {
	h, e := windows.LoadLibraryEx(name, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	return uintptr(h), e
}
func Symbol(lib uintptr, name string) (uintptr, error) {
	return windows.GetProcAddress(windows.Handle(lib), name)
}

func Close(lib uintptr) error { return windows.FreeLibrary(windows.Handle(lib)) }
