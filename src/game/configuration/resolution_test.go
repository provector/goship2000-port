package configuration

import (
	"bytes"
	"fmt"
	"gunship2000-assets/internal/userpack"
	"os"
	"path/filepath"
	"testing"
)

func TestResolutionMigration(t *testing.T) {
	for _, tt := range []struct{ runtime, want string }{
		{`{}`, "640x480"},
		{`{"simulation_render_scale":1}`, "320x200"},
		{`{"simulation_render_scale":2}`, "640x480"},
		{`{"simulation_render_scale":1,"simulation_resolution":"640x400"}`, "640x480"},
		{`{"simulation_render_scale":2,"simulation_resolution":"320x200"}`, "320x200"},
		{`{"simulation_resolution":"1024x768"}`, "1024x768"},
	} {
		path := filepath.Join(t.TempDir(), Filename)
		d, err := decodeSettings(path, []byte(fmt.Sprintf(`{"runtime":%s,"music_volume":43}`, tt.runtime)))
		if err != nil || d.Config.Runtime.SimulationResolution != tt.want || d.Config.MusicVolume != 43 {
			t.Fatal(tt, err)
		}
	}
}

func TestSavedVGAUpgradePreservesCompanions(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"runtime":{"simulation_resolution":"640x400","simulation_render_scale":2,"presentation_mode":"stepped"},"extra":{"keep":true}}`)
	if err := os.WriteFile(filepath.Join(dir, "config.cfg"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, Filename)
	doc, _, err := Initialize(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := func(name string) []byte {
		t.Helper()
		pack, err := userpack.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := pack.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	controls, gps := entry("controls.json"), entry("gps_codes.json")
	if !bytes.Equal(raw, entry("settings.json")) || doc.Config.Runtime.SimulationResolution != "640x480" {
		t.Fatal("migration must initially be in memory")
	}
	if err = doc.Save(); err != nil {
		t.Fatal(err)
	}
	doc, err = Load(path)
	if err != nil || doc.Config.Runtime.SimulationResolution != "640x480" || doc.Config.Runtime.PresentationMode != "stepped" || !bytes.Contains(entry("settings.json"), []byte(`"keep"`)) {
		t.Fatal("settings round trip", err)
	}
	if !bytes.Equal(controls, entry("controls.json")) || !bytes.Equal(gps, entry("gps_codes.json")) {
		t.Fatal("settings save changed companion contents")
	}
}
