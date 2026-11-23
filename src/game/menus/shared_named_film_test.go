package menus

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

// Use the ordinary post-Campaign film and copy only the file to an independent
// pilot/store. Exercise the actual browser controls on the receiving end.
func TestNamedFilmSharedPlayExportResume(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	campaignNaturalBaseEnd(t, c)
	ackToLibrary(t, c)
	id := c.Library.Entries[0].ID
	filmAction(t, c, 16)
	c.Library.Edit = ""
	filmLibraryFilmInput(t, c, Actions{Text: []rune("EUR City Patrol")})
	filmLibraryFilmInput(t, c, Actions{Enter: true})
	if c.Library.Entries[0].Title != "EUR City Patrol" {
		t.Fatal(c.Library.Entries, c.Library.Status)
	}
	raw, err := os.ReadFile(filepath.Join(c.FilmDirectory, "EUR City Patrol.g2r"))
	if err != nil {
		t.Fatal(err)
	}
	receiver := authoredUIController(t)
	receiver.FilmDirectory = t.TempDir()
	receiver.MissionDirectory = t.TempDir()
	before := filmJSON(receiver.Store.State())
	if err = os.WriteFile(filepath.Join(receiver.FilmDirectory, "A shared flight.G2R"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = receiver.openFilmLibrary(false, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(receiver.Library.Entries) != 1 || receiver.Library.Entries[0].ID != id || receiver.Library.Entries[0].Title != "EUR City Patrol" {
		t.Fatal(receiver.Library.Entries)
	}
	if check, err := os.ReadFile(filepath.Join(receiver.FilmDirectory, "EUR City Patrol.g2r")); err != nil || !bytes.Equal(check, raw) {
		t.Fatal("shared bytes changed", err)
	}
	filmAction(t, receiver, 19)
	missions, err := (gamestate.AuthoredStore{Directory: receiver.MissionDirectory}).List()
	if err != nil || len(missions) != 1 || missions[0].Title != "EUR City Patrol" {
		t.Fatal(missions, err, receiver.Library.Status)
	}
	filmAction(t, receiver, 15)
	if receiver.Screen != ScreenReplay {
		t.Fatal(receiver.Library.Status)
	}
	for !receiver.Viewer.Playback.Done {
		if err = receiver.Viewer.Playback.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err = receiver.AdvanceReplay(0); err != nil {
		t.Fatal(err)
	}
	filmAction(t, receiver, 15)
	if err = receiver.activateReplayControl(16); err != nil {
		t.Fatal(err)
	}
	if !receiver.Viewer.Practice || receiver.Viewer.Playback.Engine.Store != nil || !bytes.Equal(before, filmJSON(receiver.Store.State())) {
		t.Fatal("shared replay acquired career ownership")
	}
}
