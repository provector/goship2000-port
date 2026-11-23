package setupui

import (
	"os"
	"path/filepath"
	"testing"

	"gunship2000-assets/game/configuration"
)

func TestLegacyGPSPortSettingsToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), configuration.Filename)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "config.cfg"), []byte(`{"format_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := configuration.Initialize(path); err != nil {
		t.Fatal(err)
	}
	load := func() *configuration.Document {
		t.Helper()
		doc, err := configuration.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	for _, from := range []bool{false, true} {
		doc := load()
		if doc.Config.LegacyCopyProtection != from {
			t.Fatal("unexpected saved preference", from)
		}
		u := New(doc)
		u.open("settings")
		for row, s := range u.settings {
			if s.ID == "legacy_copy_protection" {
				u.activate(row)
				break
			}
		}
		if doc.Config.LegacyCopyProtection == from || load().Config.LegacyCopyProtection != from {
			t.Fatal("toggle must edit only the draft")
		}
		if err := u.saveSession(); err != nil {
			t.Fatal(err)
		}
		if load().Config.LegacyCopyProtection == from {
			t.Fatal("saved toggle did not reopen")
		}
	}
}
