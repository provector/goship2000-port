//go:build !linux && !windows

package sdlraw

import "fmt"

func libraryNames(string) []string            { return []string{"unsupported"} }
func load(string) (uintptr, error)            { return 0, fmt.Errorf("controller backend supports Linux/Windows") }
func symbol(uintptr, string) (uintptr, error) { return 0, fmt.Errorf("unsupported platform") }
func bindGUID(uintptr) func(int32) guid       { return nil }

func unload(lib uintptr) error { return nil }
