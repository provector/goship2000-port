package managedassets

import (
	"fmt"
	a "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagedKnownBadAtlasUpgradeAndFailurePreservationForUpgrade(t *testing.T) {
	old, current := os.Getenv("GS2000_OLD33_ASSETS"), os.Getenv("GS2000_R231_ASSETS")
	if old == "" || current == "" {
		t.Skip("declared immutable Preparation33 and34 required")
	}
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			root, e := os.MkdirTemp(filepath.Dir(current), ".r221-cache-")
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { os.RemoveAll(root) })
			target := filepath.Join(root, "assets")
			if e = linkAssets(old, target); e != nil {
				t.Fatal(e)
			}
			before := cacheHashes(t, target)
			// Existing directory is enough here: the converter is deliberately supplied
			// already-verified33 outputs. Actual conversion is a separate retained run.
			original := filepath.Join(root, "original", "GS2000CD")
			if e = os.MkdirAll(original, 0700); e != nil {
				t.Fatal(e)
			}
			paths := configuration.Paths{Directory: root, Original: filepath.Dir(original), Assets: target, AssetsOrigin: "default-managed"}
			calls := 0
			services := Services{Load: a.LoadFrontEndAssets, Ready: a.ValidateCurrentFrontEndCapabilities, Rename: os.Rename,
				Prepare: func(input, output string) (*a.Manifest, error) {
					calls++
					if input != paths.Original {
						t.Fatal("nested original path", input)
					}
					if failure {
						if e := os.MkdirAll(output, 0700); e != nil {
							return nil, e
						}
						return nil, fmt.Errorf("controlled map conversion failure")
					}
					if e := linkAssets(current, output); e != nil {
						return nil, e
					}
					raw, e := os.ReadFile(filepath.Join(output, "manifest.json"))
					if e != nil {
						return nil, e
					}
					var m a.Manifest
					e = a.DecodeObject(raw, &m)
					return &m, e
				},
			}
			got, e := Ensure(paths, false, io.Discard, services)
			if failure {
				if e == nil || !strings.Contains(e.Error(), "controlled map conversion failure") || got != nil {
					t.Fatal("first cause", e)
				}
				if !reflect.DeepEqual(before, cacheHashes(t, target)) {
					t.Fatal("failed upgrade changed previous complete cache")
				}
				oldReader, e := a.LoadFrontEndAssets(target)
				if e != nil || oldReader == nil {
					t.Fatal("previous cache no longer readable", e)
				}
			} else {
				if e != nil || got == nil || a.Hash(got.World.Worlds["PHIL1"].TileAtlas) != a.Phil1CleanAtlasSHA256 {
					t.Fatal("normal readiness-triggered promotion", e)
				}
				again, e := Ensure(paths, false, io.Discard, services)
				if e != nil || again == nil || calls != 1 {
					t.Fatal("valid cache must reuse", calls, e)
				}
				if logical, e := a.LogicalResourceProfile(target); e != nil || logical != a.LogicalPreparation34 {
					t.Fatal("full promoted profile", logical, e)
				}
			}
			if calls != 1 {
				t.Fatal("one-shot preparation", calls)
			}
		})
	}
}
