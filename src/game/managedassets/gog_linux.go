package managedassets

import (
	"os"
	"path/filepath"
)

func gogRoots() []string {
	home, e := os.UserHomeDir()
	if e != nil {
		return nil
	}
	wine := os.Getenv("WINEPREFIX")
	if !filepath.IsAbs(wine) {
		wine = filepath.Join(home, ".wine")
	}
	drive := filepath.Join(wine, "drive_c")
	locations := []string{
		filepath.Join(home, "GOG Games"),
		filepath.Join(home, "Games", "GOG"),
		filepath.Join(drive, "GOG Games"),
		filepath.Join(drive, "Program Files", "GOG Galaxy", "Games"),
		filepath.Join(drive, "Program Files (x86)", "GOG Galaxy", "Games"),
	}
	var out []string
	for _, dir := range locations {
		if hasGOGProduct(dir) {
			out = append(out, dir)
		}
		entries, e := os.ReadDir(dir)
		if e != nil {
			continue
		}
		for _, ent := range entries {
			path := filepath.Join(dir, ent.Name())
			if directoryExists(path) && hasGOGProduct(path) {
				out = append(out, path)
			}
		}
	}
	return out
}
