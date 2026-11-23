package setupui

import (
	"gunship2000-assets/game/configuration"
	"path/filepath"
	"testing"
)

func TestFloatingGeometryDraft(t *testing.T) {
	path := filepath.Join(t.TempDir(), configuration.Filename)
	d, _, err := configuration.Initialize(path)
	if err != nil {
		t.Fatal(err)
	}
	u := New(d)
	u.open("graphics")
	idx := -1
	for i, s := range u.settings {
		if s.ID == "floating" {
			if idx >= 0 {
				t.Fatal("duplicate")
			}
			idx = i
		}
	}
	if idx < 0 || !d.Config.Runtime.FloatingGeometry {
		t.Fatal("missing experimental switch or wrong default")
	}
	u.activate(idx)
	if d.Config.Runtime.FloatingGeometry || u.Table.GetCell(idx, 1).Text != "Off" {
		t.Fatal("toggle failed")
	}
	other, err := configuration.Load(path)
	if err != nil || !other.Config.Runtime.FloatingGeometry {
		t.Fatal("draft saved before Save & Exit", err)
	}
	if err = u.saveSession(); err != nil {
		t.Fatal(err)
	}
	other, err = configuration.Load(path)
	if err != nil || other.Config.Runtime.FloatingGeometry || other.Config.Runtime.SimulationResolution != "640x480" {
		t.Fatal("reopen", err)
	}
}
