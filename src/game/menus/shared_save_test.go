package menus

import (
	. "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func saveTestStore(t *testing.T) *SaveStore {
	t.Helper()
	s, err := OpenSaveStore(filepath.Join(t.TempDir(), "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func saveTestPilot(t *testing.T, name string) *Pilot {
	t.Helper()
	p, err := NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	p.Name = name
	p.Squadron = "Test Squadron Name"
	return p
}

func mustReadSaveBytes(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
