package sdlruntime

import "path/filepath"

// Use the framework shipped with this installation even when launched by absolute path.
func libraryNames(dir string) []string {
	return []string{filepath.Join(dir, "lib", "SDL2.framework", "Versions", "A", "SDL2")}
}
