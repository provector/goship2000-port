package main

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAssetsOverlapRefused(t *testing.T) {
	root := t.TempDir()
	for _, save := range []string{filepath.Join(root, "assets"), filepath.Join(root, "assets", "saves"), root} {
		if err := validateSaveSeparation(ApplicationPaths{Saves: save, Assets: filepath.Join(root, "assets")}); err == nil {
			t.Fatal(save)
		}
	}
}

func TestFrontEndManagedUpgradeWhenRequested(t *testing.T) {
	directory := os.Getenv("GS2000_UPGRADE_APP")
	original := os.Getenv("GS2000_STARTUP_ORIGINAL")
	if directory == "" || original == "" {
		t.Skip("explicit existing office-only upgrade fixture required")
	}
	target := filepath.Join(directory, "assets")
	before, err := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = assetdata.LoadMenuAssets(target); err == nil {
		t.Fatal("fixture already has menu metadata")
	}
	p := ApplicationPaths{Directory: directory, Original: original, Assets: target, Saves: filepath.Join(directory, "saves")}
	assets, err := ensureOfficeAssets(p, false, os.Stdout, frontEndStartupServices())
	if err != nil {
		t.Fatal(err)
	}
	if assets.Menus == nil {
		t.Fatal("upgrade is not menu-ready")
	}
	backups, _ := filepath.Glob(filepath.Join(directory, ".assets.previous-*", "tree", "manifest.json"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	preserved, err := os.ReadFile(backups[0])
	if err != nil || !bytes.Equal(before, preserved) {
		t.Fatal("previous tree not preserved")
	}
	// Fresh normal save really is empty; dev's named fixture is never substituted.
	store, err := gamestate.OpenSaveStore(p.Saves, original)
	if err != nil {
		t.Fatal(err)
	}
	if store.State().SelectedPilot() != nil {
		t.Fatal("unexpected active research fixture")
	}
	p.Original = filepath.Join(directory, "original-now-unavailable")
	if _, err = ensureOfficeAssets(p, false, os.Stdout, frontEndStartupServices()); err != nil {
		t.Fatal("upgraded assets require originals", err)
	}
}
