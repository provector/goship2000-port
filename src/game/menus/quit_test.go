package menus

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"gunship2000-assets/game/cockpit"
)

// Uses a real recorded launch/accepted-hit film. The expected surrounding pixels
// are the reached replay frame, not a screenshot manufactured by the modal.
func TestReplayQuitKeepsReachedScenePaletteAndStream(t *testing.T) {
	c, f := viewerFilm(t)
	for _, scale := range []int{1, 2} {
		for _, mode := range []string{"playing", "stopped", "demo", "map", "practice"} {
			t.Run(fmt.Sprintf("%d/%s", scale, mode), func(t *testing.T) {
				c.Runtime.SimulationRenderScale = scale
				openViewer(t, c, f)
				v := c.Viewer
				switch mode {
				case "stopped":
					v.Stopped = true
				case "demo":
					v.Demo = true
				case "map":
					v.Map = true
				case "practice":
					if err := c.activateReplayControl(16); err != nil {
						t.Fatal(err)
					}
				}
				before := append([]byte(nil), c.Frame(false)...)
				palette := append([]byte(nil), filmJSON(c.CurrentPalette())...)
				cursor := v.Playback.Cursor
				checkpoint := v.Playback.Preview().FilmCheckpoint()
				state := c.Store.State()
				token := c.Library
				if err := c.Handle(Actions{Quit: true}); err != nil {
					t.Fatal(err)
				}
				if c.Screen != ScreenQuit || c.previousScreen != ScreenReplay {
					t.Fatal("quit changed replay owner")
				}
				frame := c.Frame(false)
				if len(frame) != len(before) {
					t.Fatalf("modal replaced scene resolution %d != %d", len(frame), len(before))
				}
				expected := append([]byte(nil), before...)
				if err := cockpit.DrawSceneConfirmationScaled(expected, c.Office.Assets.Cockpit, false, scale); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(frame, expected) {
					t.Fatal("quit is not the source simulation prompt over the reached replay frame")
				}
				if !bytes.Equal(palette, filmJSON(c.CurrentPalette())) {
					t.Fatal("quit replaced replay palette")
				}
				for n := 0; n < 8; n++ {
					if err := c.Tick(); err != nil {
						t.Fatal(err)
					}
					if err := c.AdvanceReplay(.25); err != nil {
						t.Fatal(err)
					}
					c.Frame(false)
				}
				if v.Playback.Cursor != cursor || !reflect.DeepEqual(checkpoint, v.Playback.Preview().FilmCheckpoint()) {
					t.Fatal("modal advanced film")
				}
				if err := c.Handle(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				if c.Screen != ScreenReplay || c.ExitRequested || c.Library != token || !reflect.DeepEqual(state, c.Store.State()) {
					t.Fatal("cancel consumed session/career/successor")
				}
				if !bytes.Equal(before, c.Frame(false)) || !bytes.Equal(palette, filmJSON(c.CurrentPalette())) {
					t.Fatal("cancel failed to restore same scene/palette")
				}
			})
		}
	}
}

func TestReplayQuitConfirmsApplicationExitWithoutLibrarySuccessor(t *testing.T) {
	c, f := viewerFilm(t)
	openViewer(t, c, f)
	v := c.Viewer
	before := c.Store.State()
	lib := c.Library
	if err := c.Handle(Actions{Quit: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit || c.Viewer != v || c.Library != lib || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("quit was library return, career publication or next preparation")
	}
}

func TestPostMissionQuitLeavesDurableCampaignSuccessorOnce(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	campaignNaturalBaseEnd(t, c)
	ackToLibrary(t, c)
	committed := c.Store.State()
	library := c.Library
	if err := c.activateFilmControl(15); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenReplay {
		t.Fatal(c.Message)
	}
	if err := c.Handle(Actions{Quit: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Library != library || c.Mission != nil || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("quit cancellation spent completed Campaign successor")
	}
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenFilms {
		t.Fatal("viewer exit did not return library")
	}
	if err := c.exitFilmLibrary(); err != nil {
		t.Fatal(err)
	}
	next := c.Mission
	if next == nil {
		t.Fatal("legitimate library EXIT did not prepare next mission")
	}
	if err := c.exitFilmLibrary(); err != nil {
		t.Fatal(err)
	}
	if c.Mission != next || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("duplicate commit/preparation")
	}
}
