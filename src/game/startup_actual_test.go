package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This opt-in integration test exercises the ordinary config/default-assets
// owner and production preparation/loader transaction, without opening a window.
// Fixture paths are inputs to copied test apps, never selected live asset roots.
func TestOrdinaryStartupActualBundles(t *testing.T) {
	current, original := os.Getenv("GS2000_TEST_EDITOR"), os.Getenv("GS2000_STARTUP_ORIGINAL")
	if current == "" || original == "" {
		t.Skip("explicit current bundle and read-only originals required")
	}
	cases := []struct {
		name, fixture, origin string
		wantError, noOriginal bool
	}{
		{"current-default", current, "", false, true},
		{"missing-default", "", "", false, false},
		{"current-explicit", current, "explicit", false, true},
	}
	for _, v := range []struct{ name, env string }{{"23", "GS2000_STARTUP_OLD23"}, {"24", "GS2000_STARTUP_OLD24"}, {"25", "GS2000_STARTUP_OLD25"}} {
		if old := os.Getenv(v.env); old != "" {
			cases = append(cases, struct {
				name, fixture, origin string
				wantError, noOriginal bool
			}{"stale" + v.name + "-default", old, "", false, false}, struct {
				name, fixture, origin string
				wantError, noOriginal bool
			}{"stale" + v.name + "-explicit", old, "explicit", true, false})
		}
	}
	for _, v := range cases {
		t.Run(v.name, func(t *testing.T) {
			app := t.TempDir()
			originalPath := original
			if v.noOriginal {
				originalPath = filepath.Join(app, "unavailable-originals")
			}
			config, _ := json.Marshal(map[string]string{"original": originalPath})
			if e := os.WriteFile(filepath.Join(app, "config.cfg"), config, 0600); e != nil {
				t.Fatal(e)
			}
			for _, dir := range []string{"saves", "films"} {
				if e := os.Mkdir(filepath.Join(app, dir), 0755); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(filepath.Join(app, dir, "preserve"), []byte(dir+" previous state"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			target := filepath.Join(app, "assets")
			if v.fixture != "" {
				if e := copyPPreparedFixture(v.fixture, target); e != nil {
					t.Fatal(e)
				}
			}
			before, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
			override := ""
			if v.origin == "explicit" {
				override = target
			}
			p, e := loadApplication(app, override)
			if e != nil {
				t.Fatal(e)
			}
			var log bytes.Buffer
			a, e := ensureOfficeAssets(p, false, &log, frontEndStartupServices())
			if v.wantError {
				if e == nil || !strings.Contains(e.Error(), "explicitly selected tree was preserved") {
					t.Fatal("stale explicit tree was not rejected safely", e)
				}
				after, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
				if !bytes.Equal(before, after) {
					t.Fatal("explicit fixture changed")
				}
			} else {
				if e != nil {
					t.Fatal(e, log.String())
				}
				if e = assetdata.ValidateCurrentFrontEndCapabilities(a); e != nil {
					t.Fatal(e)
				}
				if before != nil && strings.HasPrefix(v.name, "stale") {
					backups, _ := filepath.Glob(filepath.Join(app, ".assets.previous-*", "tree", "manifest.json"))
					if len(backups) != 1 {
						t.Fatal("old profile not retained", backups)
					}
					old, _ := os.ReadFile(backups[0])
					if !bytes.Equal(old, before) {
						t.Fatal("retained profile rewritten")
					}
				}
			}
			for _, dir := range []string{"saves", "films"} {
				b, e := os.ReadFile(filepath.Join(app, dir, "preserve"))
				if e != nil || string(b) != dir+" previous state" {
					t.Fatal("runtime data changed", dir, e)
				}
			}
			if _, e := os.Stat(target + ".prepare.lock"); !os.IsNotExist(e) {
				t.Fatal("owned lock left behind", e)
			}
			t.Logf("ordinary selection origin=%s expected_error=%v; saved/film sentinels preserved", p.AssetsOrigin, v.wantError)
		})
	}
}

func copyPPreparedFixture(source, target string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}
		to := filepath.Join(target, rel)
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture symlink not admitted: %s", path)
		}
		if d.IsDir() {
			return os.Mkdir(to, 0755)
		}
		in, e := os.Open(path)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, in)
		closeErr := out.Close()
		if e != nil {
			return e
		}
		return closeErr
	})
}
