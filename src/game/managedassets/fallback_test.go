package managedassets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
)

func TestStoreFallbackPolicy(t *testing.T) {
	sourceError := fmt.Errorf("%w: invalid GS2000.CAT", assetdata.ErrPreparationSource)
	for _, tc := range []struct {
		name, selected string
		stores         []string
		fails          map[string]error
		attempts       []string
		cause          error
	}{
		{"steam-only", "unselected", []string{"steam"}, nil, []string{"steam"}, nil},
		{"gog-only", "unselected", []string{"gog"}, nil, []string{"gog"}, nil},
		{"both-healthy", "unselected", []string{"gog", "steam"}, nil, []string{"steam"}, nil},
		{"steam-to-gog", "steam", []string{"steam", "gog"}, map[string]error{"steam": sourceError}, []string{"steam", "gog"}, nil},
		{"gog-to-steam", "gog", []string{"steam", "gog"}, map[string]error{"gog": sourceError}, []string{"gog", "steam"}, nil},
		{"both-fail", "steam", []string{"steam", "gog"}, map[string]error{"steam": sourceError, "gog": sourceError}, []string{"steam", "gog"}, assetdata.ErrPreparationSource},
		{"custom-only", "custom", []string{"steam", "gog"}, map[string]error{"custom": sourceError}, []string{"custom"}, assetdata.ErrPreparationSource},
		{"never-use-custom", "steam", []string{"steam", "custom"}, map[string]error{"steam": sourceError}, []string{"steam"}, assetdata.ErrPreparationSource},
		{"fresh-no-stores", "unselected", []string{"custom"}, nil, nil, ErrOriginalUnavailable},
		{"disk-full", "steam", []string{"steam", "gog"}, map[string]error{"steam": syscall.ENOSPC}, []string{"steam"}, syscall.ENOSPC},
		{"output-permission", "steam", []string{"steam", "gog"}, map[string]error{"steam": os.ErrPermission}, []string{"steam"}, os.ErrPermission},
		{"cancelled", "steam", []string{"steam", "gog"}, map[string]error{"steam": context.Canceled}, []string{"steam"}, context.Canceled},
		{"internal", "steam", []string{"steam", "gog"}, map[string]error{"steam": os.ErrInvalid}, []string{"steam"}, os.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dirs := map[string]string{}
			for _, k := range []string{"steam", "gog", "custom", "unselected"} {
				dirs[k] = filepath.Join(root, k+" Żółta Gra")
				if err := os.Mkdir(dirs[k], 0700); err != nil {
					t.Fatal(err)
				}
			}
			var detected []Installation
			for _, k := range tc.stores {
				detected = append(detected, Installation{k, dirs[k]})
			}
			var tried []string
			var log bytes.Buffer
			var used string
			paths := configuration.Paths{Directory: root, Original: dirs[tc.selected], Assets: filepath.Join(root, "assets"), Config: configuration.Config{OriginalSource: tc.selected, Original: dirs[tc.selected]}}
			if tc.cause != nil && len(tc.attempts) > 0 {
				if err := os.Mkdir(paths.Assets, 0700); err != nil {
					t.Fatal(err)
				}
				previous := fmt.Sprintf(`{"generator":%q,"preparation_version":%d,"schema_version":%d}`, assetdata.PreparationGenerator, assetdata.PreparationVersion, assetdata.PreparedManifestSchema)
				if err := os.WriteFile(filepath.Join(paths.Assets, "manifest.json"), []byte(previous), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(paths.Assets, "keep"), []byte("previous"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			services := Services{
				Installations: func() []Installation { return detected },
				SourceUsed:    func(path string) { used = path },
				Rename:        os.Rename,
				Load: func(path string) (*assetdata.OfficeAssets, error) {
					for _, n := range []string{"result", "keep"} {
						if _, err := os.Stat(filepath.Join(path, n)); err == nil {
							return &assetdata.OfficeAssets{}, nil
						}
					}
					return nil, errors.New("no fixture output")
				},
				Prepare: func(input, output string) (*assetdata.Manifest, error) {
					k := strings.Split(filepath.Base(input), " ")[0]
					tried = append(tried, k)
					if _, err := os.Stat(output); !os.IsNotExist(err) {
						t.Fatalf("attempt output was reused: %s %v", output, err)
					}
					if err := os.Mkdir(output, 0700); err != nil {
						t.Fatal(err)
					}
					if err := tc.fails[k]; err != nil {
						return nil, err
					}
					if err := os.WriteFile(filepath.Join(output, "result"), []byte(k), 0600); err != nil {
						t.Fatal(err)
					}
					return &assetdata.Manifest{InputDirectory: input}, nil
				},
			}
			_, err := Ensure(paths, true, &log, services)
			if !errors.Is(err, tc.cause) || !reflect.DeepEqual(tried, tc.attempts) {
				t.Fatal(tried, err)
			}
			if err != nil {
				if used != "" {
					t.Fatal("failed source reported as used", used)
				}
				for _, k := range tc.attempts {
					if !strings.Contains(err.Error(), dirs[k]) {
						t.Fatal(err)
					}
				}
				if len(tc.attempts) > 0 {
					if b, e := os.ReadFile(filepath.Join(paths.Assets, "keep")); e != nil || string(b) != "previous" {
						t.Fatal("previous cache lost", e)
					}
				}
				return
			}
			if used != dirs[tried[len(tried)-1]] || len(tried) == 2 && !strings.Contains(log.String(), "after source failure") {
				t.Fatal(used, log.String())
			}
			services.Installations = func() []Installation { t.Fatal("warm cache triggered discovery"); return nil }
			services.Prepare = func(string, string) (*assetdata.Manifest, error) {
				t.Fatal("warm cache triggered preparation")
				return nil, nil
			}
			old := used
			used = ""
			if _, err = Ensure(paths, false, &log, services); err != nil || used != old {
				t.Fatal("warm source provenance", err, used)
			}
		})
	}
}

func TestGOGProductDataLayout(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "GD", "Gunship2000", "MPS", "GS2000CD")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(root, "goggame-1357.info")
	if err := os.WriteFile(info, []byte(`{"gameId":"1357","name":"Gunship"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !hasGOGProduct(root) || gogDataDirectory(root) != path {
		t.Fatal("recognized bundle not located")
	}
	if err := os.WriteFile(info, []byte(`{"gameId":"1357","name":"Other Game"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if hasGOGProduct(root) {
		t.Fatal("unrelated GOG product detected")
	}
}

func TestDiscoveryDeduplicatesCrossStorePhysicalPaths(t *testing.T) {
	root := t.TempDir()
	steam := filepath.Join(root, "Steam")
	gog := filepath.Join(steam, "steamapps", "common", "Bundle")
	dir := filepath.Join(gog, "GD", "Gunship2000", "MPS", "GS2000CD")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(steam, "steamapps", "appmanifest_"+steamApp+".acf")
	if err := os.WriteFile(manifest, []byte(`"AppState" { "appid" "2766090" "installdir" "Bundle" }`), 0600); err != nil {
		t.Fatal(err)
	}
	paths := installedGames([]string{steam, steam}, []string{gog, gog})
	if len(paths) != 1 || paths[0].Source != configuration.OriginalSteam || !SameInstallation(paths[0].Path, dir) {
		t.Fatal(paths)
	}
}
