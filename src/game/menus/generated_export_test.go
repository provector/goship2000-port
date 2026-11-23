package menus

import (
	"bytes"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The pilot is supplied; all mission generation, recording, editing, and
// result acknowledgement go through the production front end and file stores.
func TestGeneratedFilmLibraryEditRestartBegin(t *testing.T) {
	c := authoredUIController(t)
	c.simulationProfile = ""
	setKind := func(kind gamestate.MissionType) {
		s := c.Store.State()
		s.SelectedPilot().MissionType = kind
		if err := c.Store.Commit(s); err != nil {
			t.Fatal(err)
		}
	}
	setKind(gamestate.MissionSingle)
	brief := func() {
		for i := 0; i < 80 && c.Screen != ScreenBriefing; i++ {
			if err := c.Tick(); err != nil {
				t.Fatal(err)
			}
		}
		if c.Screen != ScreenBriefing {
			t.Fatal("briefing", c.Screen, c.Message, c.Files)
		}
	}
	if err := c.startMission(3, nil); err != nil {
		t.Fatal(err)
	}
	brief()
	startRendering(t, c)
	jFinishFade(t, c)
	originalPaths := c.Mission.Data.Paths
	initial := append([]gamestate.MissionObject(nil), c.Mission.Data.Objects...)
	for _, a := range []Actions{{}, {EndMission: true}, {}, {ConfirmQuit: true}} {
		editorAction(t, c, a)
	}
	if c.Debrief == nil {
		t.Fatal("debrief", c.Screen, c.Message, c.filmError)
	}
	film := verifyFilm(t, c)
	if !bytes.Contains(film.Launch, []byte("GeneratedSource")) {
		t.Fatal("missing placement provenance")
	}
	id := c.Debrief.Pending.Start.ID
	loopAcknowledge(t, c)
	committed := filmJSON(c.Store.State())
	if err := c.Store.CommitSortie(id); err != nil || !bytes.Equal(committed, filmJSON(c.Store.State())) {
		t.Fatal("duplicate acknowledgment", err)
	}
	if err := c.openFilmLibrary(false, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(c.Library.Entries) != 1 {
		t.Fatal("film list", c.Library.Entries)
	}
	if err := c.Library.Store.RenameEntry(c.Library.Entries[0], "Northern Patrol"); err != nil {
		t.Fatal(err)
	}
	if err := c.refreshFilmList(film.Header.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := c.activateFilmControl(19); err != nil || !strings.HasPrefix(c.Library.Status, "Exported") {
			t.Fatal("Export", err, c.Library.Status)
		}
	}
	store := gamestate.AuthoredStore{Directory: c.MissionDirectory}
	list, err := store.List()
	if err != nil || len(list) != 2 || list[0].Title != "Northern Patrol" || list[1].Title != "Northern Patrol (2)" {
		t.Fatal(list, err)
	}
	if err = c.Library.Store.DeleteEntry(c.Library.Entries[0]); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, filmJSON(c.Store.State())) {
		t.Fatal("Export changed career")
	}
	setKind(gamestate.MissionSimulated)
	if err = c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "e")
	if c.Editor == nil {
		t.Fatal("EDIT", c.Files.Status)
	}
	e := c.Editor
	for _, p := range e.Document.Placements {
		if p.Role == "primary" {
			x, y := c.editorScreen(p.X, p.Y)
			editorAction(t, c, Actions{Selector2: true, Inside: true, X: x, Y: y, PointerMoved: true})
			if !e.Held {
				t.Fatal("pickup", e.Status, x, y)
			}
			editorAction(t, c, Actions{})
			editorClick(t, c, x+2, y+2)
			break
		}
	}
	if !e.Dirty {
		t.Fatal("no geometry edit", e.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil {
		t.Fatal("SAVE", e.Status)
	}
	doc, _, err := store.Load(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(doc.Compiled.Objects, initial) && reflect.DeepEqual(doc.Compiled.Paths, originalPaths) {
		t.Fatal("geometry unchanged")
	}
	path := c.Store.Directory
	missionDir, filmDir := c.MissionDirectory, c.FilmDirectory
	a := c.Office.Assets
	if err = c.Store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gamestate.OpenSaveStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	c, err = NewFrontEnd(a, reopened)
	if err != nil {
		t.Fatal(err)
	}
	c.MissionDirectory, c.FilmDirectory = missionDir, filmDir
	if err = c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "b")
	brief()
	if !reflect.DeepEqual(c.Mission.Data.Objects, doc.Compiled.Objects) || !reflect.DeepEqual(c.Mission.Data.Paths, doc.Compiled.Paths) {
		t.Fatal("BEGIN regenerated geometry")
	}
	startRendering(t, c)
	if c.Rendering == nil || len(c.Rendering.Initialized.Flight) != 1 {
		t.Fatal("exported single-aircraft launch")
	}
	jFinishFade(t, c)
	for _, a := range []Actions{{}, {EndMission: true}, {}, {ConfirmQuit: true}} {
		editorAction(t, c, a)
	}
	if c.Debrief == nil {
		t.Fatal("edited mission result", c.Screen, c.Message, c.filmError)
	}
	child := verifyFilm(t, c)
	q, err := NewFilmPlayback(c.Office.Assets, child, c.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.Resume(); err != nil || q.Engine.Store != nil {
		t.Fatal("practice isolation", err)
	}
	loopAcknowledge(t, c)
	if dir := os.Getenv("GS2000_S2_EVIDENCE"); dir != "" {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, f := range map[string]*replay.Film{"generated": film, "edited": child} {
			data, err := replay.Marshal(f)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, name+".g2r"), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		raw, err := os.ReadFile(filepath.Join(store.Directory, doc.ID+".g2m"))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "edited.g2m"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
