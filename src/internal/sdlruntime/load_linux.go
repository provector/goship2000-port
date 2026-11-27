package sdlruntime

import "path/filepath"

func libraryNames(dir string) []string {
	return []string{filepath.Join(dir, "lib", "libSDL2-2.0.so.0"), "libSDL2-2.0.so.0"}
}
