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

func TestProgressStartsOnlyAtActualProcessing(t *testing.T) {
	for _, mode := range []string{"reuse", "recheck-reuse", "missing-original", "prepare", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			original := filepath.Join(root, "original")
			if mode != "missing-original" {
				if err := os.Mkdir(original, 0700); err != nil {
					t.Fatal(err)
				}
			}
			checks, starts, events := 0, 0, 0
			s := Services{Load: func(string) (*assets.OfficeAssets, error) {
				checks++
				if mode == "reuse" || mode == "recheck-reuse" && checks > 1 {
					return &assets.OfficeAssets{}, nil
				}
				return nil, fmt.Errorf("fixture needs preparation")
			}, Progress: func(assets.PreparationProgress) {
				events++
			}, Prepare: func(string, string) (*assets.Manifest, error) {
				if events != 1 {
					t.Errorf("processing did not have one opening event: %d", events)
				}
				starts++
				return nil, fmt.Errorf("controlled prepare failure; no predecessor promoted")
			}, Rename: os.Rename}
			paths := configuration.Paths{Directory: root, Assets: filepath.Join(root, "assets"), Original: original, AssetsOrigin: "configured-default-managed"}
			_, err := Ensure(paths, mode == "rebuild", io.Discard, s)
			if mode == "reuse" || mode == "recheck-reuse" {
				if err != nil || events != 0 || starts != 0 {
					t.Fatal(err, events, starts)
				}
			} else if err == nil {
				t.Fatal("missing controlled failure")
			}
			if mode == "missing-original" && (events != 0 || starts != 0) {
				t.Fatal("no processing/window was needed", events, starts)
			}
			if mode == "prepare" || mode == "rebuild" {
				if starts != 1 || events != 1 {
					t.Fatal(starts, events)
				}
			}
		})
	}
}
