package main

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func TestRelocatedDevelopmentRuntimeWhenRequested(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_RUNTIME")
	if dir == "" {
		t.Skip("set GS2000_TEST_RUNTIME for copied runtime validation")
	}
	paths, err := loadApplication(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = frontEndStartupServices().load(paths.Assets); err != nil {
		t.Fatal(err)
	}
	// Only a disposable save copy is opened. Loading normal data must not reconcile
	// or migrate anything in the user's relocated active save set during a test.
	copy := t.TempDir()
	err = filepath.WalkDir(paths.Saves, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		r, e := filepath.Rel(paths.Saves, p)
		if e != nil {
			return e
		}
		q := filepath.Join(copy, r)
		if d.IsDir() {
			return os.MkdirAll(q, 0700)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(q, b, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(copy, filepath.Join(t.TempDir(), "no-originals"))
	if err != nil {
		t.Fatal(err)
	}
	var before struct {
		Revision        uint64 `json:"revision"`
		SelectedPilotID string `json:"selected_pilot_id"`
	}
	raw, err := os.ReadFile(filepath.Join(paths.Saves, "roster.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	if store.State().Revision != before.Revision || store.State().SelectedPilotID != before.SelectedPilotID {
		t.Fatal("relocation lost actual selected pilot/revision")
	}
	t.Logf("Resolved runtime=%s assets=%s original=%s; retained pilot %s; audio=%+v", paths.Directory, paths.Assets, paths.Original, store.State().SelectedPilotID, paths.Config)
}
