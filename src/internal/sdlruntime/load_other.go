//go:build !linux && !windows

package sdlruntime

import "fmt"

func libraryNames(string) []string            { return []string{"unsupported"} }
func load(string) (uintptr, error)            { return 0, fmt.Errorf("controller backend supports Linux/Windows") }
func Symbol(uintptr, string) (uintptr, error) { return 0, fmt.Errorf("unsupported platform") }
func Close(lib uintptr) error                 { return nil }
