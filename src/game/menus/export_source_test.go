package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Production controllers, storage and initial reconstruction. This is not an
// OS-input or completed-objectives flight claim. Export owns authoring inputs,
// never the ending scene, historical pilot identity or continuation capability.
func TestAuthoredSourceFilmExportEditableAndTamperRejected(t *testing.T) {
	c := authoredUIController(t)
	source := authorThroughUI(t, c)
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if e := c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("BEGIN", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	editorAction(t, c, Actions{})
	if e := c.advanceFlight(.1); e != nil {
		t.Fatal(e)
	}
	if c.Screen == ScreenQuit {
		c.cancelNaturalSortieEnd()
		c.Screen = ScreenScene
		editorAction(t, c, Actions{})
	}
	editorAction(t, c, Actions{EndMission: true})
	editorAction(t, c, Actions{ConfirmQuit: true})
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal("result/film", c.Screen, c.Message, c.filmError)
	}
	film, e := c.Library.Store.Load(c.Library.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	var launch filmLaunch
	if e = json.Unmarshal(film.Launch, &launch); e != nil {
		t.Fatal(e)
	}
	if launch.AuthoredSource == nil || launch.AuthoredSource.ID != source.ID {
		t.Fatal("source not frozen")
	}
	q, e := NewFilmPlayback(c.Office.Assets, film, c.Runtime)
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	corrupt := *film
	altered, e := gamestate.CloneAuthored(launch.AuthoredSource)
	if e != nil {
		t.Fatal(e)
	}
	altered.Placements[0].X++
	bad := launch
	bad.AuthoredSource = altered
	corrupt.Launch = filmJSON(bad)
	if _, e = NewFilmPlayback(c.Office.Assets, &corrupt, c.Runtime); e == nil || !strings.Contains(e.Error(), "authored source") {
		t.Fatal("forged authoring accepted", e)
	}
	corrupt = *film
	corrupt.Header.Simulation = replay.P4AuthoredCompatibility
	want := "P5 authored"
	if film.R1Mechanics() {
		// New R1/v10 cannot be relabelled as P4: the explicit pair rejects
		// before reaching the older authored-source carrier guard.
		want = "simulation or input version"
	}
	if _, e = NewFilmPlayback(c.Office.Assets, &corrupt, c.Runtime); e == nil || !strings.Contains(e.Error(), want) {
		t.Fatal("new carrier admitted historically", e)
	}
	career := filmJSON(c.Store.State())
	token := c.Library.Return
	path := publishedFilmPath(t, c, film.Header.ID)
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.exportFilmToMission(); e != nil {
		t.Fatal(e)
	}
	if c.Library.Return != token || !bytes.Equal(career, filmJSON(c.Store.State())) {
		t.Fatal("Export acquired career/successor")
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("film changed", e)
	}
	entries, e := (gamestate.AuthoredStore{Directory: c.MissionDirectory}).List()
	if e != nil || len(entries) != 2 {
		t.Fatal(entries, e)
	}
	var doc *gamestate.AuthoredDocument
	var rev string
	for _, entry := range entries {
		if entry.ID != source.ID {
			doc, rev, e = (gamestate.AuthoredStore{Directory: c.MissionDirectory}).Load(entry.ID)
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if doc == nil || doc.Initial != nil || !reflect.DeepEqual(doc.Placements, source.Placements) || doc.ID == source.ID {
		t.Fatal("not an independent editable descriptor graph")
	}
	if e = c.editMissionDocument(doc, rev); e != nil {
		t.Fatal(e)
	}
	editorKey(t, c, "t")
	editorKey(t, c, "7")
	editorAction(t, c, Actions{Enter: true})
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil || c.Screen != ScreenMissionFiles {
		t.Fatal("edited Export resave", c.Screen, c.Editor)
	}
	saved, _, e := c.Files.Store.Load(doc.ID)
	if e != nil || saved.Initial != nil {
		t.Fatal("reopen descriptor graph", e)
	}
	if e = c.beginAuthoredMission(saved); e != nil {
		t.Fatal("edited Export BEGIN", e)
	}
	if c.Mission.PilotID != c.Store.State().SelectedPilot().ID {
		t.Fatal("Export adopted historical pilot")
	}
}
