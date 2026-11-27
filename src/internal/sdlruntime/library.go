// Package sdlruntime resolves the SDL library shared by raw input and SETUP.
// Each opener owns its loader reference until all bound functions are unused.
package sdlruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Open() (uintptr, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	var failures []error
	for _, name := range libraryNames(filepath.Dir(exe)) {
		h, e := load(name)
		if e == nil {
			return h, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", name, e))
	}
	return 0, fmt.Errorf("SDL2 unavailable: %w", errors.Join(failures...))
}
