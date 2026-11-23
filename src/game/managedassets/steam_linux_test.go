package managedassets

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestNativeAndFlatpakRoots(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	d := filepath.Join(h, "xdg")
	t.Setenv("XDG_DATA_HOME", d)
	roots := steamRoots()
	for _, p := range []string{filepath.Join(d, "Steam"), filepath.Join(h, ".steam", "steam"), filepath.Join(h, ".var", "app", "com.valvesoftware.Steam", "data", "Steam")} {
		if !slices.Contains(roots, p) {
			t.Fatal(p)
		}
	}
}
