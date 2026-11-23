package main

import (
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingConfigurationInitializesPackBeforeAssetsOrCareer(t *testing.T) {
	dir := t.TempDir()
	legacy := []byte(`{"music_volume":37}`)
	os.WriteFile(filepath.Join(dir, "config.json"), legacy, 0600)
	paths, err := loadApplication(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if paths.Config.MusicVolume != 100 {
		t.Fatal("used obsolete config.json")
	}
	if _, err := os.Stat(filepath.Join(dir, configuration.Filename)); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"config.cfg", "assets", "saves", "films", "missions"} {
		if _, e := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(e) {
			t.Fatal("missing cfg caused mutation", n, e)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	if string(b) != string(legacy) {
		t.Fatal("legacy changed")
	}
}
