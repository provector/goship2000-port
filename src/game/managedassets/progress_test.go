package managedassets

import (
	"fmt"
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedRebuildPreservesPredecessorAndNeverFinishes(t *testing.T) {
	for _, failure := range []string{"prepare", "promotion", "final-read"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "assets")
			original := filepath.Join(root, "original")
			os.Mkdir(original, 0700)
			os.Mkdir(target, 0700)
			m := assets.Manifest{Generator: assets.PreparationGenerator, PreparationVersion: 31, SchemaVersion: assets.PreparedManifestSchema}
			raw, _ := assets.JsonBytes(m)
			os.WriteFile(filepath.Join(target, "manifest.json"), raw, 0600)
			os.WriteFile(filepath.Join(target, "sentinel"), []byte("previous"), 0600)
			ready := false
			var work assets.PreparationWork
			s := Services{Progress: func(p assets.PreparationProgress) {
				p = work.Observe(p)
				if p.Fraction() == 1 || p.Stage == "Prepared cache ready" {
					ready = true
				}
			}, Prepare: func(_, out string) (*assets.Manifest, error) {
				if failure == "prepare" {
					return nil, fmt.Errorf("injected extraction failure")
				}
				os.Mkdir(out, 0700)
				os.WriteFile(filepath.Join(out, "manifest.json"), raw, 0600)
				return &m, nil
			}, Load: func(p string) (*assets.OfficeAssets, error) {
				if failure == "final-read" && p == target {
					return nil, fmt.Errorf("injected final readback failure")
				}
				return &assets.OfficeAssets{}, nil
			}, Rename: func(a, b string) error {
				if failure == "promotion" && b == target && filepath.Base(a) == "tree" && !filepath.IsAbs(filepath.Base(a)) { // only generated tree, not restoration
					if data, e := os.ReadFile(filepath.Join(a, "sentinel")); e != nil || string(data) != "previous" {
						return fmt.Errorf("injected promotion failure")
					}
				}
				return os.Rename(a, b)
			}}
			p := configuration.Paths{Directory: root, Original: original, Assets: target, AssetsOrigin: "configured-default-managed"}
			_, e := Ensure(p, true, io.Discard, s)
			if e == nil || ready {
				t.Fatal(e, ready)
			}
			if failure != "final-read" {
				data, e := os.ReadFile(filepath.Join(target, "sentinel"))
				if e != nil || string(data) != "previous" {
					t.Fatal("predecessor lost", e)
				}
			} else {
				rows, _ := filepath.Glob(filepath.Join(root, ".assets.previous-*", "tree", "sentinel"))
				if len(rows) != 1 {
					t.Fatal("predecessor backup lost", rows)
				}
			}
		})
	}
}
