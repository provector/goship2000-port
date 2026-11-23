package main

import (
	"bytes"
	"errors"
	assetdata "gunship2000-assets/game/assets"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetSelectionOriginPreserved(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{}`), 0600)
	p, e := loadApplication(dir, "")
	if e != nil || p.AssetsOrigin != "default-managed" {
		t.Fatal(p, e)
	}
	dir = t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{"assets":"assets"}`), 0600)
	p, e = loadApplication(dir, "")
	if e != nil || p.AssetsOrigin != "configured-default-managed" {
		t.Fatal(p, e)
	}
	dir = t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{"assets":"historical/preparation25"}`), 0600)
	p, e = loadApplication(dir, "")
	if e != nil || p.AssetsOrigin != "configured" {
		t.Fatal(p, e)
	}
	p, e = loadApplication(dir, "frozen")
	if e != nil || p.AssetsOrigin != "command-line" {
		t.Fatal(p, e)
	}
}
func TestCurrentCapabilityUpgradePreservesPriorTree(t *testing.T) {
	p, s, count := fakeStartup(t)
	if _, e := ensureOfficeAssets(p, false, io.Discard, s); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(p.Assets, "historical-profile"), []byte("valid film resource profile"), 0600)
	// The historical loader succeeds, but current UI is a separate contract.
	s.ready = func(a *assetdata.OfficeAssets) error {
		b, e := os.ReadFile(filepath.Join(a.Root, "current-ui"))
		if e != nil || string(b) != "editor/replay" {
			return errors.New("Mission Editor screen is missing or outdated")
		}
		return nil
	}
	prepare := s.prepare
	s.prepare = func(a, b string) (*assetdata.Manifest, error) {
		m, e := prepare(a, b)
		if e == nil {
			e = os.WriteFile(filepath.Join(b, "current-ui"), []byte("editor/replay"), 0600)
		}
		return m, e
	}
	var log bytes.Buffer
	if _, e := ensureOfficeAssets(p, false, &log, s); e != nil {
		t.Fatal(e)
	}
	if *count != 2 {
		t.Fatal("compatible stale tree was incorrectly reused", *count)
	}
	old, _ := filepath.Glob(filepath.Join(p.Directory, ".assets.previous-*", "tree", "historical-profile"))
	if len(old) != 1 {
		t.Fatal("historical resource profile not retained", old)
	}
	if _, e := ensureOfficeAssets(p, false, io.Discard, s); e != nil || *count != 2 {
		t.Fatal("current tree regenerated", e, *count)
	}
}
func TestExplicitStaleAssetsNeverWrite(t *testing.T) {
	for _, origin := range []string{"command-line", "configured"} {
		t.Run(origin, func(t *testing.T) {
			p, s, count := fakeStartup(t)
			if _, e := ensureOfficeAssets(p, false, io.Discard, s); e != nil {
				t.Fatal(e)
			}
			p.AssetsOrigin = origin
			s.ready = func(*assetdata.OfficeAssets) error { return errors.New("Mission Films text missing") }
			before, _ := os.ReadFile(filepath.Join(p.Assets, "manifest.json"))
			if _, e := ensureOfficeAssets(p, false, io.Discard, s); e == nil || !strings.Contains(e.Error(), "explicitly selected tree was preserved") || !strings.Contains(e.Error(), p.Assets) {
				t.Fatal(e)
			}
			after, _ := os.ReadFile(filepath.Join(p.Assets, "manifest.json"))
			if !bytes.Equal(before, after) || *count != 1 {
				t.Fatal("explicit reference replaced")
			}
			if _, e := os.Stat(p.Assets + ".prepare.lock"); !os.IsNotExist(e) {
				t.Fatal("explicit error created lock", e)
			}
		})
	}
}
func TestStaleManagedWithoutOriginalsKeepsCache(t *testing.T) {
	p, s, count := fakeStartup(t)
	ensureOfficeAssets(p, false, io.Discard, s)
	p.Original = filepath.Join(p.Directory, "missing-originals")
	s.ready = func(*assetdata.OfficeAssets) error { return errors.New("Mission Editor screen missing") }
	before, _ := os.ReadFile(filepath.Join(p.Assets, "manifest.json"))
	if _, e := ensureOfficeAssets(p, false, io.Discard, s); e == nil || !strings.Contains(e.Error(), "original installation unavailable") {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(filepath.Join(p.Assets, "manifest.json"))
	if !bytes.Equal(before, after) || *count != 1 {
		t.Fatal("stale cache changed without originals")
	}
}
