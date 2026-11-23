package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

// Real controller authoring -> stored/reopened source Deep Strike -> normal
// preflight -> actual collective/cyclic inputs -> reached corridor boundary.
// No pose, goal, RNG, ending or result field is assigned. This is controller
// integration, not physical OS input or human continuous-flight acceptance.
func TestDeepStrikeControlsNaturalCorridorResultReplayRestart(t *testing.T) {
	c := authoredUIController(t)
	doc := authorDeepStrikeUI(t, c)
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
	p := c.Rendering
	r := p.Flight
	initialPose := r.State.Pose
	initialPilot := filmJSON(c.Store.State().SelectedPilot())
	initialCount := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter
	if p.Initialized.Switches&0x800 == 0 || p.filmRecorder == nil {
		t.Fatal("actual recorded corridor launch missing", p.Initialized.Switches, p.filmRecorder, c.filmError, c.Message)
	}
	editorAction(t, c, Actions{}) // Genuine release after the preflight handoff.
	for i := 0; i < 8; i++ {
		editorAction(t, c, Actions{FlightCommands: []int{0x100}, SceneDirections: [4]bool{false, false, false, true}})
	}
	var timeline []map[string]any
	for i := 0; i < 3000 && c.Screen == ScreenScene; i++ {
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal("actual controls", i, e)
		}
		if i%50 == 0 || c.Screen != ScreenScene {
			timeline = append(timeline, map[string]any{"iteration": i, "steps": r.Steps, "pose": r.State.Pose, "collective": r.State.Collective, "height_target": r.State.HeightTarget, "height_control": r.State.HeightControl, "forward": r.State.Forward, "stage": r.State.FlightStage, "boundary": r.State.BoundaryCrossed, "pending": p.Initialized.Sortie.Pending, "screen": c.Screen})
		}
	}
	if out := os.Getenv("GS2000_P5_ENTRY_ARTIFACTS"); out != "" {
		if e := os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
		b, _ := json.MarshalIndent(map[string]any{"document": doc.ID, "initial_pose": initialPose, "timeline": timeline, "scope": "real controller controls, not supplied pose/goals/ending or OS flight"}, "", "  ")
		if e := os.WriteFile(filepath.Join(out, "natural-corridor-timeline.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenQuit || !c.sceneEndConfirmation || p.Initialized.Sortie.Pending != "corridor" || r.State.BoundaryCrossed&1 == 0 {
		t.Fatalf("natural corridor not reached: screen%s pose%+v forward%d collective%d heightcontrol%d terminal%+v pending%s timeline%v", c.Screen, r.State.Pose, r.State.Forward, r.State.Collective, r.State.HeightControl, r.State.Terminal, p.Initialized.Sortie.Pending, timeline)
	}
	if !bytes.Equal(initialPilot, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("career changed before result")
	}
	sessionID := c.Mission.ResultLaunch.Start.ID
	editorAction(t, c, Actions{ConfirmQuit: true})
	if c.Screen != ScreenDebrief || c.Debrief == nil {
		t.Fatal("natural result rejected", c.Screen, c.Message)
	}
	if !bytes.Equal(initialPilot, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("natural result published beforefinalack")
	}
	for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
		editorAction(t, c, Actions{Enter: true})
		editorAction(t, c, Actions{MenuEnd: true})
		editorAction(t, c, Actions{Enter: true})
	}
	if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
		t.Fatal("natural finalack/film", c.Screen, c.Message, c.filmError)
	}
	film, e := c.Library.Store.Load(c.Library.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	if film.End.Reason != "corridor" {
		t.Fatal("film ending is not reached corridor", film.End)
	}
	for _, scale := range []int{1, 2} {
		cfg := c.Runtime
		cfg.SimulationRenderScale = scale
		play, e := NewFilmPlayback(c.Office.Assets, film, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for !play.Done {
			if e = play.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if play.Engine.Store != nil {
			t.Fatal("replay gained career authority")
		}
	}
	if got := c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter; got != initialCount+1 {
		t.Fatal("raw2 natural exit did not commit ordinary source mission counter", initialCount, got)
	}
	committed := filmJSON(c.Store.State())
	if e = c.Store.CommitSortie(sessionID); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(committed, filmJSON(c.Store.State())) {
		t.Fatal("duplicate commit changed career")
	}
	library := c.Library
	editorAction(t, c, Actions{Cancel: true})
	if c.Screen != ScreenOffice || c.Mission != nil || !library.Return.Consumed {
		t.Fatal("raw2 library successor", c.Screen, c.Mission, library.Return)
	}
	if e := c.exitFilmLibrary(); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(committed, filmJSON(c.Store.State())) {
		t.Fatal("library exit repeated career publication")
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(committed, filmJSON(reopened.State())) {
		t.Fatal("restart changed committed result")
	}
	pending, e := reopened.PendingSortie()
	if e != nil || pending != nil {
		t.Fatal("restart retainedpending", pending, e)
	}
	next, e := NewFrontEnd(c.Office.Assets, reopened)
	if e != nil {
		t.Fatal(e)
	}
	if next.Screen != ScreenOffice || next.Mission != nil {
		t.Fatal("restart resurrected authored flight/result", next.Screen, next.Mission)
	}
	if out := os.Getenv("GS2000_P5_ENTRY_ARTIFACTS"); out != "" {
		b, e := os.ReadFile(publishedFilmPath(t, c, film.Header.ID))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "natural-corridor.g2r"), b, 0600); e != nil {
			t.Fatal(e)
		}
		b, _ = json.MarshalIndent(map[string]any{"header": film.Header, "ending": film.End, "operations": len(film.Operations), "initial_pose": initialPose, "last_pose": r.State.Pose, "steps": r.Steps, "session": sessionID, "scope": "actual UI-authored/reopened entry, real controls and natural corridor; no preset goals; strict ownfilm/finalack/restart"}, "", "  ")
		if e = os.WriteFile(filepath.Join(out, "natural-corridor-result.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("actual source corridor initial%+v reached%+v steps%d operations%d score%d", initialPose, r.State.Pose, r.Steps, len(film.Operations), film.End.Score)
}

// Independent source service operand check, not a flown boundary substitute.
func TestDeepStrikeCorridorMetricEquality(t *testing.T) {
	for _, delta := range []int32{4095, 4096, 4097} {
		base := gamestate.MissionPoint{X: 0x4000 + 16*delta, Y: 291672}
		pose := flight.Pose{H0: 0x4000, H1: uint32(base.Y), Height: 300}
		if got := gamestate.SourceCorridorEligible(base, pose, 1); got != (delta < 4096) {
			t.Fatal("source shifted16 strict boundary", delta, got)
		}
		if gamestate.SourceCorridorEligible(base, pose, 0) {
			t.Fatal("proximity fabricated crossing", delta)
		}
	}
}
