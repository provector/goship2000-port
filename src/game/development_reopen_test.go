//go:build devtools

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReopenScenarioUsesOnlyOwnedScratchPointer(t *testing.T) {
	app := t.TempDir()
	owned := filepath.Join(app, "dev-output", "menu-save-test")
	if err := os.MkdirAll(owned, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(owned, "roster.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	write := func(path string) {
		raw, _ := json.Marshal(map[string]string{"scratch_save": path})
		if err := os.WriteFile(filepath.Join(app, "dev-output", "scenario.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(owned)
	if got, err := reopenScenarioSave(app); err != nil || got != owned {
		t.Fatal(got, err)
	}
	write(t.TempDir())
	if _, err := reopenScenarioSave(app); err == nil {
		t.Fatal("external pointer admitted")
	}
	if err := os.WriteFile(filepath.Join(app, "dev-output", "scenario.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopenScenarioSave(app); err == nil {
		t.Fatal("corrupt optional pointer admitted")
	}
}
