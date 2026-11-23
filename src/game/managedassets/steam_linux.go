package managedassets

import (
	"os"
	"path/filepath"
)

func steamRoots() []string {
	home, e := os.UserHomeDir()
	if e != nil {
		return nil
	}
	roots := []string{filepath.Join(home, ".steam", "steam"), filepath.Join(home, ".steam", "root"), filepath.Join(home, ".local", "share", "Steam")}
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		roots = append(roots, filepath.Join(d, "Steam"))
	}
	flat := filepath.Join(home, ".var", "app", "com.valvesoftware.Steam")
	return append(roots, filepath.Join(flat, ".local", "share", "Steam"), filepath.Join(flat, "data", "Steam"), filepath.Join(flat, ".steam", "steam"))
}
