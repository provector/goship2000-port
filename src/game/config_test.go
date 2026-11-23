package main

import (
	"bytes"
	"gunship2000-assets/internal/userpack"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredTerrainOptionLoadsWithoutRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.cfg")
	raw := []byte(`{"music_enabled":false,"sound_effects_volume":0,"runtime":{"terrain_pattern_mode":"world","simulation_render_scale":2}}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := loadApplication(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.MusicEnabled || got.Config.SoundEffectsVolume != 0 || got.Config.Runtime.SimulationRenderScale != 2 || got.Config.Runtime.PresentationFPS != 60 {
		t.Fatal(got.Config)
	}
	after, err := userpack.ReadConfig(dir, "settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, after) {
		t.Fatal("configuration was silently rewritten")
	}
	bad := []byte(`{"runtime":{"terrain_pattern_mdoe":"world"}}`)
	dir = t.TempDir()
	path = filepath.Join(dir, "config.cfg")
	os.WriteFile(path, bad, 0600)
	// Q1.4 deliberately changed the missing-field default to genuine 2x.
	// An unrelated retired/unknown key must inherit that default, not the old1x.
	if got, err := loadApplication(dir, ""); err != nil || got.Config.Runtime.SimulationRenderScale != 2 {
		t.Fatal("unrelated cfg key should be retained without becoming a setting", err)
	}
	after, err = userpack.ReadConfig(dir, "settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, bad) {
		t.Fatal("unknown configuration field rewritten")
	}
}
