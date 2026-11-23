package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// Controller/action integration only. It runs the shipped editor, persistence,
// browser and normal preflight consumers; it is not an OS-input receipt.
func authorDeepStrikeUI(t *testing.T, c *FrontEnd) *gamestate.AuthoredDocument {
	t.Helper()
	doc := authorThroughUI(t, c)
	editorKey(t, c, "e")
	editorKey(t, c, "b")
	deep := -1
	for i, o := range c.Editor.Options {
		if o.Name == "Deep Strike" {
			deep = i
			break
		}
	}
	if deep < 0 {
		t.Fatal("Deep Strike source choice absent")
	}
	r := c.Editor.popupArea(deep, len(c.Editor.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	editorClick(t, c, 8, 60)
	if c.Editor.Held {
		t.Fatal("entry held command failed", c.Editor.Status, c.Editor.ShadowError)
	}
	var entry *gamestate.AuthoredPlacement
	for i := range c.Editor.Document.Placements {
		if c.Editor.Document.Placements[i].Entry != nil {
			entry = &c.Editor.Document.Placements[i]
		}
	}
	if entry == nil || entry.X != 60 || entry.Y != 513 {
		t.Fatal("source edge shadow/accepted placement", entry, c.Editor.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil || c.Screen != ScreenMissionFiles {
		t.Fatal("Deep Strike SAVE", c.Screen, c.Editor)
	}
	saved, _, err := c.Files.Store.Load(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Compiled == nil || saved.Compiled.Entry == nil || saved.Compiled.Conditions.Flags&0x800 == 0 {
		t.Fatal("saved source entry missing")
	}
	// Restart the production front-end on the same disposable store, not a reset.
	next, err := NewFrontEnd(c.Office.Assets, c.Store)
	if err != nil {
		t.Fatal(err)
	}
	next.MissionDirectory, next.FilmDirectory = c.MissionDirectory, c.FilmDirectory
	*c = *next
	if err = c.OpenMissionFiles(); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "e")
	if c.Editor.Document.ID != saved.ID || c.Editor.Document.Compiled.Entry == nil {
		t.Fatal("reopened EDIT lost entry")
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil || c.Screen != ScreenMissionFiles {
		t.Fatal("unchanged resave", c.Screen, c.Editor)
	}
	return saved
}
func TestDeepStrikeUIEditSaveRestartReopenBegin(t *testing.T) {
	c := authoredUIController(t)
	doc := authorDeepStrikeUI(t, c)
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("Deep Strike BEGIN", c.Screen, c.Message, c.Files.Status)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Rendering == nil || c.Rendering.Initialized == nil || c.Rendering.Initialized.Switches&0x800 == 0 {
		t.Fatal("actual initialization not corridor")
	}
	m := c.Rendering.Initialized
	base := doc.Compiled.Objectives["base"]
	// This five-row pickup/drop fixture selects source formation2: row10 of
	// the11-byte formation table offsets actor0 byY+800 before3D entry.
	if m.Flight[0].Group != 0 || m.Flight[4].Group == 0 {
		t.Fatal("unexpected source flight formation selection", m.Flight)
	}
	if m.Player.H0 != uint32(base.Point.X) || m.Player.H1 != uint32(base.Point.Y+800) || m.InitialForwardMotion != 120 || m.InitialHeightTarget != 300 {
		t.Fatal("source entry not frozen at launch", m.Player, m.InitialForwardMotion, m.InitialHeightTarget)
	}
	t.Logf("actual BEGIN source pose%+v forward%d heighttarget%d switches%04x", m.Player, m.InitialForwardMotion, m.InitialHeightTarget, m.Switches)
}
