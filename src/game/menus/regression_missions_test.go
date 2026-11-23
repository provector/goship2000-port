package menus

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	state "gunship2000-assets/game/state"
)

// Build fixtures exclusively from the same choices and placement functions exposed by the editor.
func regressionDocument(t *testing.T, c *FrontEnd, title, world string) *state.AuthoredDocument {
	t.Helper()
	data := c.Office.Assets.Missions.Generation
	d, err := state.NewAuthoredDocument(data, world, title, 41)
	if err != nil {
		t.Fatal(err)
	}
	w, err := state.AuthoredEditorWorld(data, world)
	if err != nil {
		t.Fatal(err)
	}
	place := func(role string, kind, x, y int) {
		options, e := state.AuthoredOptions(data, d, role, kind)
		if e != nil || len(options) == 0 {
			t.Fatalf("%s options: %v (%d)", role, e, len(options))
		}
		if e = state.PlaceAuthored(data, d, role, kind, options[0].Formation, x, y); e != nil {
			t.Fatal(role, e)
		}
	}
	baseX, baseY := 800, 500
	destX, destY := 860, 550
	if world == "EURO9.WRL" {
		longest := 0
		for y := 2; y < 62; y++ {
			begin := 2
			for x := 2; x <= 62; x++ {
				if x < 62 && w.LAND[w.Grid[64*y+x]&63]&4 != 0 {
					continue
				}
				if x-begin > longest {
					longest = x - begin
					baseX, baseY = (begin-1)*24+12, y*18+9
					destX, destY = x*24+12, baseY
				}
				begin = x + 1
			}
		}
		if longest < 2 {
			t.Fatal("no city crossing")
		}
		t.Logf("city strip from (%d,%d) to (%d,%d), %d squares", baseX, baseY, destX, destY, longest)
	}
	place("base", 0, baseX, baseY)
	// Fixed base options snap to the source site. Anchor nearby pickup tasks to that actual base.
	if title != "City and Return" {
		for _, p := range d.Placements {
			if p.Role == "base" {
				baseX, baseY = p.X, p.Y
			}
		}
		destX, destY = min(1450, baseX+85), min(1070, baseY+60)
	}
	place("primary", 7, destX, destY)
	place("secondary", 5, min(1500, destX+84), min(1100, destY+60))
	if title == "FARP and Jungle" {
		place("FARP", 0, min(1450, baseX+120), min(1050, baseY+90))
	}
	if title == "Moving and Support" {
		opts, _, e := state.AuthoredEditorOptions(data, d, "defender", 0, 2, true)
		if e != nil {
			t.Fatal(e)
		}
		moving := false
		for _, o := range opts {
			flags := binary.LittleEndian.Uint32(w.FMHD[o.Formation*39+35:])
			if o.Unavailable != "" || flags&0x180 == 0 {
				continue
			}
			if e = state.PlaceAuthoredOption(data, d, "defender", 0, o, 400, 400); e != nil {
				t.Fatal(e)
			}
			unit := d.Placements[len(d.Placements)-1]
			if e = state.PlaceAuthored(data, d, "primary", 2, unit.Formation, unit.X, unit.Y); e != nil {
				t.Fatal(e)
			}
			if e = state.SetAuthoredRoutePoint(data, d, "primary", 600, 400); e != nil {
				t.Fatal(e)
			}
			moving = true
			break
		}
		if !moving {
			t.Fatal("no moving group")
		}
	}
	if title == "Moving and Support" || title == "FARP and Jungle" {
		opts, _, e := state.AuthoredEditorOptions(data, d, "defender", 0, 0, false)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, o := range opts {
			if o.Unavailable == "" && (strings.Contains(o.Name, "Hind") || strings.Contains(o.Name, "Hip") || strings.Contains(o.Name, "MD-530")) {
				if e = state.PlaceAuthoredOption(data, d, "defender", 0, o, destX, destY); e != nil {
					t.Fatal(e)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no usable enemy helicopter in editor")
		}
	}
	if title == "Moving and Support" {
		opts, e := state.AuthoredSupportOptions(data, d)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, o := range opts {
			if o.Cost == 1 && o.Unavailable == "" {
				if e = state.SetAuthoredSupport(data, d, o); e != nil {
					t.Fatal(e)
				}
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no air support")
		}
	}
	if err = state.ValidateAuthored(data, d); err != nil {
		t.Fatal(err)
	}
	for _, p := range d.Placements {
		if !p.Builtin {
			t.Logf("%s: formation %d at %d,%d", p.Role, p.Formation, p.X, p.Y)
		}
	}
	return d
}

func TestRegressionMissionEditorFlightRoundTrip(t *testing.T) {
	for _, scenario := range []struct{ title, world string }{
		{"City and Return", "EURO9.WRL"}, {"FARP and Jungle", "PHIL2.WRL"}, {"Moving and Support", "EURO6.WRL"}, {"End and Recovery", "EURO6.WRL"},
	} {
		t.Run(scenario.title, func(t *testing.T) {
			c := authoredUIController(t)
			c.simulationProfile = ""
			d := regressionDocument(t, c, scenario.title, scenario.world)
			theatre := state.TheatreCentralEurope
			if scenario.world == "PHIL2.WRL" {
				theatre = state.TheatrePhilippines
				career := c.Store.State()
				career.SelectedPilot().Theatre = theatre
				if e := c.Store.Commit(career); e != nil {
					t.Fatal(e)
				}
			}
			_, e := state.CompileAuthoredMission(c.editorData(), d, state.MissionInputs{MissionType: state.MissionSimulated, Theatre: theatre, Rank: state.RankCaptain, PilotName: "Editor test", Seed: int64(d.Seed)})
			if e != nil {
				t.Fatal(e)
			}
			store := state.AuthoredStore{Directory: c.MissionDirectory}
			if _, e := store.Save(d, ""); e != nil {
				t.Fatal(e)
			}
			if e := c.OpenMissionFiles(); e != nil {
				t.Fatal(e)
			}
			editorKey(t, c, "e")
			if c.Editor == nil || c.Editor.Document.ID != d.ID {
				t.Fatalf("editor selection: %s %+v", c.Screen, c.Files)
			}
			// Re-place the secondary objective through the normal editor draft and save it using the UI.
			doc := c.Editor.Document
			for _, p := range doc.Placements {
				if p.Role == "secondary" {
					if e := state.PlaceAuthored(c.Office.Assets.Missions.Generation, doc, "secondary", p.Kind, p.Formation, min(1500, p.X+24), p.Y); e != nil {
						t.Fatal(e)
					}
					break
				}
			}
			editorKey(t, c, "x")
			editorKey(t, c, "s")
			if c.Screen != ScreenMissionFiles {
				t.Fatalf("SAVE: %s %+v", c.Screen, c.Editor)
			}
			saved, _, e := store.Load(d.ID)
			if e != nil || saved.Title != d.Title {
				t.Fatal("reopen", e)
			}
			next, e := NewFrontEnd(c.Office.Assets, c.Store)
			if e != nil {
				t.Fatal(e)
			}
			next.MissionDirectory = c.MissionDirectory
			next.FilmDirectory = c.FilmDirectory
			c = next
			t.Cleanup(c.AbandonFilmRecording)
			if e = c.OpenMissionFiles(); e != nil {
				t.Fatal(e)
			}
			editorKey(t, c, "b")
			for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
				if e = c.Tick(); e != nil {
					t.Fatal(e)
				}
			}
			if c.Screen != ScreenBriefing {
				t.Fatalf("BEGIN: %s %s %+v", c.Screen, c.Message, c.Files)
			}
			startRendering(t, c)
			jFinishFade(t, c)
			for n := 0; n < 12; n++ {
				if e = c.advanceFlight(.1); e != nil {
					t.Fatal(e)
				}
			}
			if c.Screen == ScreenQuit {
				c.cancelNaturalSortieEnd()
				c.Screen = ScreenScene
			}
			editorAction(t, c, Actions{EndMission: true})
			editorAction(t, c, Actions{ConfirmQuit: true})
			if c.Screen != ScreenDebrief {
				t.Fatalf("ending: %s %s", c.Screen, c.Message)
			}
			for n := 0; n < 64 && c.Screen != ScreenFilms; n++ {
				editorAction(t, c, Actions{Enter: true})
				editorAction(t, c, Actions{MenuEnd: true})
				editorAction(t, c, Actions{Enter: true})
			}
			if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
				t.Fatal("film publication", c.Screen, c.Message)
			}
			film, e := c.Library.Store.Load(c.Library.Entries[0].ID)
			if e != nil {
				t.Fatal(e)
			}
			before := filmJSON(c.Store.State())
			q, e := NewFilmPlayback(c.Office.Assets, film, c.Runtime)
			if e != nil {
				t.Fatal(e)
			}
			for !q.Done {
				if e = q.Step(); e != nil {
					t.Fatal(e)
				}
			}
			if q.Divergence != "" {
				t.Fatal(q.Divergence)
			}
			q, e = NewFilmPlayback(c.Office.Assets, film, c.Runtime)
			if e != nil {
				t.Fatal(e)
			}
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
			if e = q.Resume(); e != nil {
				t.Fatal(e)
			}
			if e = q.AdvancePractice(.1); e != nil {
				t.Fatal(e)
			}
			if q.Engine.Store != nil || !bytes.Equal(before, filmJSON(c.Store.State())) {
				t.Fatal("practice career ownership")
			}
			reopened, e := state.OpenSaveStore(c.Store.Directory, "")
			if e != nil {
				t.Fatal(e)
			}
			if reopened.State().SelectedPilot().Statistics.MissionCampaignCounter != 13 {
				t.Fatal("restart did not retain exactly one result")
			}
			if out := os.Getenv("GS2000_REGRESSION_MISSION_OUTPUT"); out != "" {
				if e = os.MkdirAll(out, 0700); e != nil {
					t.Fatal(e)
				}
				b, e := os.ReadFile(filepath.Join(store.Directory, d.ID+".g2m"))
				if e != nil {
					t.Fatal(e)
				}
				f, e := os.OpenFile(filepath.Join(out, d.Title+".g2m"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if e != nil {
					t.Fatal(e)
				}
				_, writeErr := f.Write(b)
				closeErr := f.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatal(writeErr, closeErr)
				}
			}
		})
	}
}
