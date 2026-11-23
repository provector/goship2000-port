package main

import (
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCampaignImportMaintenanceCreatesSeparateSave(t *testing.T) {
	assets := os.Getenv("GS2000_TEST_MISSIONS")
	if assets == "" {
		t.Skip("set GS2000_TEST_MISSIONS for import/readiness integration")
	}
	dir := testfixture.Path("captain-observation")
	source := t.TempDir()
	paths := map[string]string{}
	for _, name := range []string{"ROSTER.DAT", "CAMPGN09.DAT", "CAMPGN03.DAT"} {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			t.Skip("user comparison files absent")
		}
		if err != nil {
			t.Fatal(err)
		}
		paths[name] = filepath.Join(source, name)
		if err = os.WriteFile(paths[name], raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "imported-save")
	if err := importCampaignSave(assets, paths["ROSTER.DAT"], paths["CAMPGN09.DAT"], []string{paths["CAMPGN03.DAT"]}, output, 1); err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(output, "")
	if err != nil {
		t.Fatal(err)
	}
	p := store.State().SelectedPilot()
	if p == nil || p.Name != "Test" || p.Campaign == nil || p.Campaign.Phase != gamestate.CampaignSuspended {
		t.Fatal("maintenance import did not preserve the selected supplied pilot")
	}
	raw, err := os.ReadFile(filepath.Join(output, store.State().Legacy.PreservedPath))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(paths["ROSTER.DAT"])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(raw, want) {
		t.Fatal("raw roster import changed")
	}
	before := assetdata.Hash(testSaveBytes(t, store.Directory, "roster.json"))
	if err = importCampaignSave(assets, paths["ROSTER.DAT"], paths["CAMPGN09.DAT"], nil, output, 1); err == nil {
		t.Fatal("maintenance import overwrote existing save")
	}
	if assetdata.Hash(testSaveBytes(t, store.Directory, "roster.json")) != before {
		t.Fatal("failed repeat changed loaded save")
	}
}
