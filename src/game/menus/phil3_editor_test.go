package menus

import (
	gamestate "gunship2000-assets/game/state"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPhilippinesThirdEditorCreateSaveReopenAndFly(t *testing.T) {
	c := authoredUIController(t)
	state := c.Store.State()
	state.SelectedPilot().Theatre = gamestate.TheatrePhilippines
	if err := c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	d := authorWorldThroughUI(t, c, "PHIL3")
	if editorMapKey(d.World) != "PHIL3" {
		t.Fatal("wrong new world", d.World)
	}
	next, err := NewFrontEnd(c.Office.Assets, c.Store)
	if err != nil {
		t.Fatal(err)
	}
	next.MissionDirectory = c.MissionDirectory
	next.FilmDirectory = c.FilmDirectory
	c = next
	t.Cleanup(c.AbandonFilmRecording)
	if err = c.OpenMissionFiles(); err != nil {
		t.Fatal(err)
	}
	if err = c.activateMissionFile(19); err != nil || c.Editor == nil {
		t.Fatal("EDIT", err)
	}
	if dir := os.Getenv("GS2000_W3_CAPTURE"); dir != "" {
		pic := image.NewPaletted(image.Rect(0, 0, 320, 200), c.CurrentPalette())
		copy(pic.Pix, c.Frame(false))
		file, err := os.Create(filepath.Join(dir, "phil3-editor.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, pic)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if err = c.activateMissionFile(15); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	startRendering(t, c)
	if c.Rendering == nil || editorMapKey(c.Rendering.Initialized.World) != "PHIL3" {
		t.Fatal("wrong flight world", c.Screen, c.Message)
	}
	data := c.Mission.Data
	ex, err := gamestate.NewAuthoredFromMission(c.editorData(), data, "Third map export", d.Seed)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	rev, err := c.Files.Store.Save(ex, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.editMissionDocument(ex, rev); err != nil || c.Editor == nil || editorMapKey(c.Editor.Document.World) != "PHIL3" {
		t.Fatal("exported edit", err)
	}
}
