package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyDocumentFixture(t *testing.T, c *FrontEnd, env string) *gamestate.AuthoredDocument {
	t.Helper()
	path := os.Getenv(env)
	if path == "" {
		t.Skip("P6 document prerequisite: " + env)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(c.MissionDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(c.MissionDirectory, filepath.Base(path)), b, 0600); err != nil {
		t.Fatal(err)
	}
	s := gamestate.AuthoredStore{Directory: c.MissionDirectory}
	d, _, err := s.Load(strings.TrimSuffix(filepath.Base(path), ".g2m"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMissionFilesTheatreFilterUsesDocumentIdentityForEveryAction(t *testing.T) {
	c := authoredUIController(t)
	europe := copyDocumentFixture(t, c, "GS2000_P5_AUTHORED_DOC")
	ant := copyDocumentFixture(t, c, "GS2000_P6_SEARCH_DOCUMENT")
	other := copyDocumentFixture(t, c, "GS2000_P6_EURO_OTHER_DOCUMENT")
	if europe.World == other.World {
		t.Fatal("different maps in one theatre prerequisite")
	}
	store := gamestate.AuthoredStore{Directory: c.MissionDirectory}
	for i := 0; i < 11; i++ {
		q, _ := gamestate.CloneAuthored(europe)
		q.ID, q.Title = fmt.Sprintf("%032x", i+1), fmt.Sprintf("List%02d", i)
		q.Compiled.Revision = gamestate.AuthoredRevision(q)
		if _, err := store.Save(q, ""); err != nil {
			t.Fatal(err)
		}
	}
	unknown, _ := gamestate.CloneAuthored(europe)
	unknown.ID, unknown.World, unknown.Title = strings.Repeat("f", 32), "UNKNOWN.WRL", "Unknown"
	unknown.Compiled.Revision = gamestate.AuthoredRevision(unknown)
	if _, err := store.Save(unknown, ""); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(c.MissionDirectory, strings.Repeat("e", 32)+".g2m")
	if err := os.WriteFile(badPath, []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	antPath := filepath.Join(c.MissionDirectory, ant.ID+".g2m")
	antBefore, _ := os.ReadFile(antPath)
	if err := c.openMissionFiles(); err != nil || len(c.Files.Entries) != 13 || c.Files.FilterNotice == "" {
		t.Fatal("theatre-level filter/unknown notice", err, len(c.Files.Entries), c.Files.FilterNotice)
	}
	f := c.Files
	for _, e := range f.Entries {
		if e.ID == ant.ID || e.ID == unknown.ID {
			t.Fatal("wrong/unknown theatre leaked into list")
		}
	}
	// Paging addresses the filtered list. Selection must survive title sorting.
	if err := c.activateMissionFile(14); err != nil {
		t.Fatal(err)
	}
	selected := f.Entries[f.Selected].ID
	if err := c.activateMissionFile(16); err != nil {
		t.Fatal(err)
	}
	f.Edit = "AAA renamed"
	if err := c.handleMissionFiles(Actions{Enter: true}); err != nil || f.Entries[f.Selected].ID != selected {
		t.Fatal("rename changed selected identity", err)
	}
	loaded, _, err := store.Load(selected)
	if err != nil || loaded.Title != "AAA renamed" {
		t.Fatal("rename acted on wrong document", err)
	}
	if err := c.activateMissionFile(19); err != nil || c.Editor == nil || c.Editor.Document.ID != selected {
		t.Fatal("EDIT used unfiltered index", err)
	}
	c.Editor, c.Screen = nil, ScreenMissionFiles
	if err := c.activateMissionFile(17); err != nil || f.DeleteID != selected {
		t.Fatal("DELETE used unfiltered index", err)
	}
	if err := c.handleMissionFiles(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(selected); err != nil {
		t.Fatal("cancel deleted selected mission", err)
	}
	if err := c.activateMissionFile(17); err != nil {
		t.Fatal(err)
	}
	if err := c.handleMissionFiles(Actions{Text: []rune("y")}); err != nil || len(f.Entries) != 12 {
		t.Fatal("confirmed delete", err, len(f.Entries))
	}
	if _, _, err := store.Load(selected); !os.IsNotExist(err) {
		t.Fatal("confirmed delete did not remove exact displayed ID", err)
	}
	// An operating-theatre change cancels the old action; it cannot BEGIN the
	// new theatre's first row using a key sampled in the old list.
	state := c.Store.State()
	state.SelectedPilot().Theatre = gamestate.TheatreAntarctica
	if err := c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	if err := c.handleMissionFiles(Actions{Enter: true}); err != nil || c.Screen != ScreenMissionFiles || len(f.Entries) != 1 || f.Entries[0].ID != ant.ID || f.Focus != 21 {
		t.Fatal("theatre switch retained stale action/index", err)
	}
	if err := c.activateMissionFile(15); err != nil || c.Mission == nil || c.Mission.AuthoredSource.ID != ant.ID {
		t.Fatal("BEGIN did not use filtered document", err, f.Status)
	}
	after, _ := os.ReadFile(antPath)
	if !bytes.Equal(antBefore, after) {
		t.Fatal("browsing/BEGIN rewrote a retained document")
	}
	if _, err := os.Stat(badPath); err != nil {
		t.Fatal("unknown/truncated file was removed")
	}
}

func savedMissionPath(t *testing.T, dir, id string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.g2m"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var header struct{ ID string }
		if json.Unmarshal(b, &header) == nil && header.ID == id {
			return path
		}
	}
	t.Fatalf("no saved mission %q in %s", id, dir)
	return ""
}
