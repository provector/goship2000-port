package menus

import (
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPreflightWritableComparisonSaveIdentities(t *testing.T) {
	source := os.Getenv("GS2000_TEST_D3_SAVE")
	if source == "" {
		t.Skip("set GS2000_TEST_D3_SAVE to the writable comparison save")
	}
	// Clone the advertised working save again: repeatable tests never consume
	// the user's manual comparison state, immutable fixture or normal progress.
	dest := filepath.Join(t.TempDir(), "saves")
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	c := missionController(t)
	c.Store, err = gamestate.OpenSaveStore(dest, filepath.Join(t.TempDir(), "absent-original"))
	if err != nil {
		t.Fatal(err)
	}
	state := c.Store.State()
	p := state.SelectedPilot()
	if p == nil || p.Name != "Test Pilot Name" || p.Rank != gamestate.RankCaptain {
		t.Fatal("wrong comparison pilot")
	}
	p.MissionType = gamestate.MissionFlight
	if err = c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	readyMission(t, c)
	s := c.Mission
	for i, name := range []string{"Arthur", "Adams", "Anderson", "Bennett"} {
		if s.Flight[i+1].Name != name || s.Flight[i+1].Portrait != i+1 {
			t.Fatal("fixture identity", i, s.Flight[i+1])
		}
	}
	if err = c.openOutfit(); err != nil {
		t.Fatal(err)
	}
	v := s.Outfit
	before := append([]gamestate.FlightPilot(nil), s.Flight...)
	v.Mode, v.Source, v.Selected = "pilot", 1, 2
	if err = v.apply(); err != nil {
		t.Fatal(err)
	}
	if s.Flight[1].Name != before[2].Name || s.Flight[1].Portrait != before[2].Portrait || s.Flight[2].Name != before[1].Name {
		t.Fatal("pilot transfer did not move identity")
	}
	pilots := append([]gamestate.FlightPilot(nil), s.Flight...)
	rows := append([][36]byte(nil), s.ReadinessRows...)
	v.Mode, v.Source, v.Selected, v.Duplicate = "helicopter", 1, 3, false
	if err = v.apply(); err != nil {
		t.Fatal(err)
	}
	if s.ReadinessRows[1] != rows[3] || s.ReadinessRows[3] != rows[1] || !reflect.DeepEqual(pilots, s.Flight) {
		t.Fatal("helicopter swap changed pilots or not loadout")
	}
	v.Mode, v.Source, v.Selected, v.Duplicate = "helicopter", 0, 4, true
	if err = v.apply(); err != nil {
		t.Fatal(err)
	}
	if s.ReadinessRows[4] != s.ReadinessRows[0] || !reflect.DeepEqual(pilots, s.Flight) {
		t.Fatal("duplication changed pilots")
	}
	if err = c.openOrdnance(4); err != nil {
		t.Fatal(err)
	}
	ordnanceActivate(t, s.Ordnance, "back", -1)
	if err = c.handleLoadout(Actions{}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOutfit || !reflect.DeepEqual(pilots, s.Flight) || s.ReadinessRows[4] != s.ReadinessRows[0] {
		t.Fatal("round trip lost assignment/equipment")
	}
}
