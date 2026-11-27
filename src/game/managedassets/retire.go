package managedassets

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gunship2000-assets/game/assets"
)

// Only retire an entirely identified generated predecessor after the new
// generation has passed activation. A mod, unrelated file or symlink preserves it.
func retireGenerated(root string) error {
	raw, e := assets.ReadLimitedFile(filepath.Join(root, "manifest.json"), 64<<20)
	if e != nil {
		return e
	}
	var m assets.Manifest
	if e = json.Unmarshal(raw, &m); e != nil {
		return e
	}
	if m.Generator != assets.PreparationGenerator {
		return fmt.Errorf("unidentified generator")
	}
	expected := map[string]string{"manifest.json": assets.Hash(raw)}
	if _, b := preparedSource(root); b != nil {
		expected[usedSourceFile] = assets.Hash(b)
	}
	if _, e = os.Lstat(filepath.Join(root, "packs.json")); e == nil {
		// The verified packed layout also owns the catalog and three data packs.
		if _, e = assets.LogicalResourceProfile(root); e != nil {
			return e
		}
		for _, n := range []string{"packs.json", "core.g2pak", "intro.g2pak", "audio.g2pak"} {
			expected[n] = ""
		}
	} else if os.IsNotExist(e) {
		for _, r := range m.Resources {
			if r == nil {
				return fmt.Errorf("invalid resource")
			}
			for _, o := range r.Outputs {
				expected[o.Path] = o.SHA256
			}
		}
	} else {
		return e
	}
	dirs := map[string]bool{".": true}
	for name := range expected {
		for parent := filepath.Dir(filepath.FromSlash(name)); parent != "."; parent = filepath.Dir(parent) {
			if filepath.IsAbs(parent) || parent == ".." {
				return fmt.Errorf("unsafe predecessor manifest path")
			}
			dirs[parent] = true
		}
	}
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		name, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if d.IsDir() {
			if !dirs[name] {
				return fmt.Errorf("unowned predecessor directory: %s", name)
			}
			return nil
		}
		sum, ok := expected[filepath.ToSlash(name)]
		if !d.Type().IsRegular() || !ok {
			return fmt.Errorf("unowned predecessor file: %s", name)
		}
		if sum != "" {
			b, e := assets.ReadLimitedFile(path, 96<<20)
			if e != nil || assets.Hash(b) != sum {
				return fmt.Errorf("modified predecessor: %s", name)
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	return os.RemoveAll(root)
}
