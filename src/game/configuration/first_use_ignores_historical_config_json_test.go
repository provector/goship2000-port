package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstUseIgnoresHistoricalConfigJSON(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "config.json")
	b := []byte(`{"music_volume":3}`)
	os.WriteFile(old, b, 0600)
	p, e := LoadApplication(dir, "")
	if e != nil || p.Config.MusicVolume != DefaultConfig().MusicVolume {
		t.Fatal(p.Config.MusicVolume, e)
	}
	if data, e := os.ReadFile(old); e != nil || !bytes.Equal(b, data) {
		t.Fatal("changed historical file", e)
	}
}
func TestFirstCreationReadNoWriteAndDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), Filename)
	d, created, e := Initialize(p)
	if e != nil || !created {
		t.Fatal(created, e)
	}
	b, _ := os.ReadFile(p)
	if !bytes.Contains(b, []byte("\n  \"format_version\"")) {
		t.Fatal("not pretty JSON")
	}
	if d.Config.MusicVolume != 100 || d.Config.SoundEffectsVolume != 100 || !d.Config.SpeechEnabled || d.Config.Runtime.SimulationRenderScale != 2 || d.Config.Runtime.PresentationFPS != 60 || d.Config.ReplayMaxMiB != 128 {
		t.Fatal(d.Config)
	}
	_, created, e = Initialize(p)
	after, _ := os.ReadFile(p)
	if e != nil || created || !bytes.Equal(b, after) {
		t.Fatal("open changed file", e)
	}
}
func TestEditableTextUnknownKeysAndStepped30Roundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.cfg")
	raw := []byte(`{ "music_volume":37,"future":{"number":9007199254740993},"runtime":{"presentation_mode":"stepped","presentation_fps":30,"future_visual":42},"ui":{"render_scale":{"label":"My wording","tooltip":"My help","choices":{"2":{"label":"My high mode","tooltip":"Choice help","future":9}}}},"audio":{"music_mode":"background_off","future_backend":"unchanged"}}`)
	os.WriteFile(p, raw, 0600)
	d, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	same, _ := os.ReadFile(p)
	if !bytes.Equal(raw, same) {
		t.Fatal("open rewrote bytes")
	}
	d.Config.FadeMilliseconds = 88
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p)
	var root map[string]json.RawMessage
	json.Unmarshal(b, &root)
	if !bytes.Contains(b, []byte("9007199254740993")) || !bytes.Contains(b, []byte("future_visual")) || !bytes.Contains(b, []byte("future_backend")) || !bytes.Contains(b, []byte("Choice help")) || !bytes.Contains(b, []byte("\"future\": 9")) {
		t.Fatal(string(b))
	}
	q, e := Load(p)
	if e != nil || q.Config.Runtime.PresentationMode != "stepped" || q.Config.Runtime.PresentationFPS != 30 || q.Config.MusicVolume != 37 || q.Config.UI["render_scale"].Label != "My wording" {
		t.Fatal(q, e)
	}
}
func TestAtomicConflictReadOnlyAndInvalidPreserve(t *testing.T) {
	p := filepath.Join(t.TempDir(), Filename)
	d, _, _ := Initialize(p)
	stale, _ := Load(p)
	d.Config.MusicVolume = 37
	d.Save()
	before, _ := os.ReadFile(p)
	stale.Config.MusicVolume = 12
	if e := stale.Save(); !errors.Is(e, ErrChanged) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("conflict changed file")
	}
	d, _ = Load(p)
	d.Config.ReplayMaxMiB = 1025
	if e := d.Save(); e == nil {
		t.Fatal("invalid budget saved")
	}
	after, _ = os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid save changed file")
	}
	d, _ = Load(p)
	os.Chmod(p, 0400)
	defer os.Chmod(p, 0600)
	if e := d.Save(); e == nil || !strings.Contains(e.Error(), "read-only") {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("readonly changed")
	}
}
func TestInvalidTypesLocationsAndBoundaries(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.cfg")
	for _, s := range []string{"{\n broken}", `null`, `[]`, `{} {}`, `{"music_volume":null}`, `{"runtime":null}`, `{"audio":{"music_mode":null}}`, `{"music_volume":"37"}`, `{"music_volume":100.1}`, `{"window_width":319}`, `{"replay_max_mib":0}`, `{"replay_max_mib":1025}`, `{"runtime":{"simulation_render_scale":3}}`, `{"assets":"same","saves":"same/inside"}`, `{"speech_enabled":"off"}`, `{"format_version":2}`, `{"music_volume":37,"music_volume":90}`} {
		os.WriteFile(p, []byte(s), 0600)
		if _, e := Load(p); e == nil || !strings.Contains(e.Error(), p) {
			t.Fatalf("accepted %q: %v", s, e)
		}
	}
	os.WriteFile(p, []byte("{\n broken}"), 0600)
	if _, e := Load(p); e == nil || !strings.Contains(e.Error(), "line 2 column") {
		t.Fatal(e)
	}
	for _, n := range []int{1, 64, 128, 256, 1024} {
		c := DefaultConfig()
		c.ReplayMaxMiB = n
		if e := c.Validate(); e != nil {
			t.Fatal(n, e)
		}
	}
}
func TestSliderToggleLastPositiveAndDefault(t *testing.T) {
	c := DefaultConfig()
	c.SetVolume(true, 37)
	c.SetVolume(true, 0)
	if c.MusicEnabled || c.LastMusicVolume != 37 {
		t.Fatal(c)
	}
	c.ToggleChannel(true)
	if !c.MusicEnabled || c.MusicVolume != 37 {
		t.Fatal(c)
	}
	c.ToggleChannel(true)
	c.ToggleChannel(true)
	if c.MusicVolume != 37 {
		t.Fatal(c)
	}
	c.LastEffectsVolume = 0
	c.SoundEffectsVolume = 0
	c.SoundEffectsEnabled = false
	c.ToggleChannel(false)
	if c.SoundEffectsVolume != 100 || !c.SoundEffectsEnabled {
		t.Fatal(c)
	}
	c.Audio.MusicMode = MusicOff
	c.ToggleChannel(true)
	if c.Audio.MusicMode != MusicAll || !c.MusicEnabled {
		t.Fatal("legacy off didnot enable", c)
	}
}
func TestBothExecutablesSameResolverAndSourceDefaults(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src")
	os.MkdirAll(filepath.Join(source, "game"), 0700)
	os.Mkdir(filepath.Join(source, "runtime"), 0700)
	os.WriteFile(filepath.Join(source, "go.mod"), []byte("module gunship2000-assets\n"), 0600)
	os.WriteFile(filepath.Join(source, "game/main.go"), []byte("package main"), 0600)
	for _, exe := range []string{"game", "setup"} {
		p, e := ApplicationDirectory("", filepath.Join(source, "runtime"), filepath.Join(root, exe), "gunship2000-assets", "auto")
		if e != nil || p != filepath.Join(source, "runtime") || DefaultConfig().Original != "original" {
			t.Fatal(p, e)
		}
	}
	for _, exe := range []string{"game", "setup"} {
		p, e := ApplicationDirectory("", t.TempDir(), filepath.Join(root, exe), "gunship2000-assets", "installed")
		if e != nil || p != root {
			t.Fatal(p, e)
		}
	}
}
