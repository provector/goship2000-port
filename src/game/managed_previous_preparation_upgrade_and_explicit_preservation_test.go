package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

// The accepted P.2 cache and the retained early P.3 candidate load historically,
// but lack new current UI owners. They are inputs, never upgrade destinations.
func TestManagedPreviousPreparationUpgradeAndExplicitPreservation(t *testing.T) {
	original := os.Getenv("GS2000_STARTUP_ORIGINAL")
	for _, env := range []string{"GS2000_P3_PREPARATION27", "GS2000_P3_CANDIDATE28", "GS2000_P6_OLD29"} {
		fixture := os.Getenv(env)
		if original == "" || fixture == "" {
			t.Skip("hash-bound previous preparation and originals required: " + env)
		}
		for _, explicit := range []bool{false, true} {
			name := env + "/managed"
			if explicit {
				name = env + "/explicit"
			}
			t.Run(name, func(t *testing.T) {
				app := t.TempDir()
				target := filepath.Join(app, "assets")
				if err := copyPPreparedFixture(fixture, target); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(filepath.Join(target, "manifest.json"))
				if err != nil {
					t.Fatal(err)
				}
				cfg, _ := json.Marshal(map[string]string{"original": original})
				if err = os.WriteFile(filepath.Join(app, "config.cfg"), cfg, 0600); err != nil {
					t.Fatal(err)
				}
				for _, d := range []string{"saves", "films", "missions"} {
					if err = os.Mkdir(filepath.Join(app, d), 0700); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(app, d, "preserve"), []byte(d), 0600); err != nil {
						t.Fatal(err)
					}
				}
				override := ""
				if explicit {
					override = target
				}
				p, err := loadApplication(app, override)
				if err != nil {
					t.Fatal(err)
				}
				var log bytes.Buffer
				a, err := ensureOfficeAssets(p, false, &log, frontEndStartupServices())
				if explicit {
					if err == nil || !strings.Contains(err.Error(), "explicitly selected tree was preserved") {
						t.Fatal("expected explicit old UI rejection", err)
					}
					after, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
					if !bytes.Equal(before, after) {
						t.Fatal("explicit predecessor changed")
					}
				} else {
					if err != nil {
						t.Fatal(err, log.String())
					}
					if err = assetdata.ValidateCurrentFrontEndCapabilities(a); err != nil {
						t.Fatal(err)
					}
					backups, _ := filepath.Glob(filepath.Join(app, ".assets.previous-*", "tree", "manifest.json"))
					if len(backups) != 1 {
						t.Fatal("old profile not retained", backups)
					}
					old, _ := os.ReadFile(backups[0])
					if !bytes.Equal(before, old) {
						t.Fatal("historical resource manifest rewritten")
					}
					current, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
					p.Original = filepath.Join(app, "unavailable")
					if _, err = ensureOfficeAssets(p, false, &log, frontEndStartupServices()); err != nil {
						t.Fatal("current relaunch required originals", err)
					}
					again, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
					if !bytes.Equal(current, again) {
						t.Fatal("upgrade loop")
					}
				}
				for _, d := range []string{"saves", "films", "missions"} {
					b, e := os.ReadFile(filepath.Join(app, d, "preserve"))
					if e != nil || string(b) != d {
						t.Fatal("runtime state changed", d, e)
					}
				}
				if _, e := os.Stat(target + ".prepare.lock"); !os.IsNotExist(e) {
					t.Fatal("lock remains", e)
				}
			})
		}
	}
}
