//go:build windows

package replay

// File data is flushed before portable rename. Directory fsync is unavailable
// through os.File on Windows; crash durability of metadata is not claimed by
// compile-only Windows delivery. Ordinary write/rename errors remain explicit.
func syncFilmDirectory(path string) error { return nil }
