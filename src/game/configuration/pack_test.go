package configuration

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/internal/userpack"
	"os"
	"path/filepath"
	"testing"
)

func TestPackedConfigMigrationAndIndependentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	raw := []byte(`{"replay_max_mib":1024,"retained":{"untouched":true}}`)
	if e := os.WriteFile(filepath.Join(dir, "config.cfg"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	gps := []byte(`{"code":"00123"}`)
	os.WriteFile(filepath.Join(dir, "gps_codes.json"), gps, 0600)
	ctrl, _ := json.Marshal(controllers.Defaults())
	os.WriteFile(filepath.Join(dir, "controls.cfg"), ctrl, 0600)
	d, _, e := Initialize(path)
	if e != nil {
		t.Fatal(e)
	}
	if d.Config.ReplayMaxMiB != 1024 {
		t.Fatal("record limit lost")
	}
	c, e := controllers.Load(path)
	if e != nil {
		t.Fatal(e)
	}
	other, _, e := Initialize(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = userpack.Locked(path, func() error {
		return userpack.PatchLocked(path, []userpack.Change{{Name: "careers/test.json", After: []byte("{}")}})
	}); e != nil {
		t.Fatal(e)
	}
	d.Config.ReplayMaxMiB = 321
	if e = d.SaveTogether(c); e != nil {
		t.Fatal(e)
	}
	other.Config.ReplayMaxMiB = 15
	if e = other.Save(); e == nil {
		t.Fatal("stale settings replaced pack")
	}
	got, _, e := Initialize(path)
	if e != nil || got.Config.ReplayMaxMiB != 321 {
		t.Fatal(e)
	}
	a, e := userpack.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	settings, _ := a.Get("settings.json")
	g, _ := a.Get("gps_codes.json")
	if !bytes.Equal(g, gps) || !bytes.Contains(settings, []byte("untouched")) {
		t.Fatal("unrelated config lost")
	}
	if _, e = a.Get("careers/test.json"); e != nil {
		t.Fatal(e)
	}
}
