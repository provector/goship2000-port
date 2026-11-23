package main

import (
	sound "gunship2000-assets/game/sounds"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioConfigOmittedEnableDefaultsAndInvalidValues(t *testing.T) {

	for _, contents := range []string{"", `{"window_width":640}`, `{"music_volume":50}`} {
		d := t.TempDir()
		if contents != "" {
			if e := os.WriteFile(filepath.Join(d, "config.cfg"), []byte(contents), 0600); e != nil {
				t.Fatal(e)
			}
		}
		p, e := loadApplication(d, "")
		if e != nil {
			t.Fatal(e)
		}
		if !p.Config.MusicEnabled || !p.Config.SoundEffectsEnabled {
			t.Fatal("omitted enable switches must default true")
		}
	}
	for _, contents := range []string{`{"audio":{"music_mode":"MIDI"}}`, `{"sound_effects_volume":101}`, `{"music_volume":-1}`, `{"music_volume":100.5}`, `{"music_enabled":"no"}`} {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "config.cfg"), []byte(contents), 0600)
		if _, e := loadApplication(d, ""); e == nil {
			t.Fatalf("accepted %s", contents)
		}
	}
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "config.cfg"), []byte(`{"music_enabled":false,"sound_effects_enabled":false}`), 0600)
	p, e := loadApplication(d, "")
	if e != nil || p.Config.MusicEnabled || p.Config.SoundEffectsEnabled {
		t.Fatal("explicit false not retained", e)
	}
}

func TestAudioConfiguredIntegerGainsAndMutePreservesVolume(t *testing.T) {
	for _, tc := range []struct {
		json           string
		music, effects int
	}{
		{`{}`, 100, 100},
		{`{"music_volume":0}`, 0, 100},
		{`{"music_volume":25,"sound_effects_volume":50}`, 25, 50},
		{`{"sound_effects_volume":0}`, 100, 0},
	} {
		d := t.TempDir()
		if e := os.WriteFile(filepath.Join(d, "config.cfg"), []byte(tc.json), 0600); e != nil {
			t.Fatal(e)
		}
		paths, e := loadApplication(d, "")
		if e != nil {
			t.Fatal(e)
		}
		if paths.Config.MusicVolume != tc.music || paths.Config.SoundEffectsVolume != tc.effects {
			t.Fatal(paths.Config)
		}
		p, e := sound.NewAudioPlayer(audioTestAssets(), *paths.Config.audioPreferences())
		if e != nil {
			t.Fatal(e)
		}
		p.Sync("office", nil, false)
		p.PlayCue(0x22)
		out := make([]float32, 1)
		p.Render(out, 1)
		want := (float32(1000)*float32(tc.music)/100 + float32(100)*float32(tc.effects)/100) / 32768
		if out[0] != want {
			t.Fatalf("gain %s: %v want %v", tc.json, out[0], want)
		}
		p.SetMusicEnabled(false)
		p.SetSoundEffectsEnabled(false)
		p.Render(out, 1)
		if out[0] != 0 {
			t.Fatal("mute not silent")
		}
		p.SetMusicEnabled(true)
		p.SetSoundEffectsEnabled(true)
		p.PlayCue(0x22)
		p.Render(out, 1)
		if out[0] != want {
			t.Fatal("mute discarded stored gains")
		}
	}
}
