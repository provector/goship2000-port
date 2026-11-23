package setupui

import (
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"testing"
)

func TestWingmenToggleSaveDiscardAndOldDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), configuration.Filename)
	// A valid older configuration has no AI consumer key.
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "config.cfg"), []byte(`{"format_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := configuration.Initialize(path); err != nil {
		t.Fatal(err)
	}
	doc, err := configuration.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Config.Runtime.DisableWingmenAI {
		t.Fatal("missing setting must default Off")
	}
	before, _ := os.ReadFile(path)
	ui := New(doc)
	ui.open("debug")
	ui.activate(2)
	if !doc.Config.Runtime.DisableWingmenAI {
		t.Fatal("preview did not become a toggle")
	}
	// Discard closes the session without invoking the document writer.
	reopened, err := configuration.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Config.Runtime.DisableWingmenAI {
		t.Fatal("draft escaped without Save")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("opening/toggling mutated file")
	}
	if err = doc.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err = configuration.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Config.Runtime.DisableWingmenAI {
		t.Fatal("Save/reopen lost On")
	}
	ui = New(reopened)
	ui.open("debug")
	ui.activate(2)
	if err = reopened.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err = configuration.Load(path)
	if err != nil || reopened.Config.Runtime.DisableWingmenAI {
		t.Fatal("Off persistence", err)
	}
	if Catalogue("debug")[1].Kind != "toggle" {
		t.Fatal("R2 stationary Enemy AI toggle missing")
	}
}
