package setupui

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"testing"
)

func TestEnemyToggleOldDefaultSaveDiscardIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), configuration.Filename)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "config.cfg"), []byte(`{"format_version":1,"runtime":{"disable_wingmen_ai":true},"private_note":"retain"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := configuration.Initialize(path); err != nil {
		t.Fatal(err)
	}
	doc, err := configuration.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Config.Runtime.DisableEnemyAI || !doc.Config.Runtime.DisableWingmenAI {
		t.Fatal("old default / independent policies")
	}
	before, _ := os.ReadFile(path)
	u := New(doc)
	u.open("debug")
	u.activate(1)
	if !doc.Config.Runtime.DisableEnemyAI || !doc.Config.Runtime.DisableWingmenAI {
		t.Fatal("reachable enemy toggle")
	}
	other, err := configuration.Load(path)
	if err != nil || other.Config.Runtime.DisableEnemyAI {
		t.Fatal("draft escaped without Save", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("opening changed existing bytes")
	}
	if err = doc.Save(); err != nil {
		t.Fatal(err)
	}
	other, err = configuration.Load(path)
	if err != nil || !other.Config.Runtime.DisableEnemyAI || !other.Config.Runtime.DisableWingmenAI {
		t.Fatal("Save/reopen", err)
	}
	u = New(other)
	u.open("debug")
	u.activate(1)
	if err = other.Save(); err != nil {
		t.Fatal(err)
	}
	other, err = configuration.Load(path)
	if err != nil || other.Config.Runtime.DisableEnemyAI {
		t.Fatal("Off persistence", err)
	}
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte("retain")) {
		t.Fatal("unrelated configuration lost")
	}
}
