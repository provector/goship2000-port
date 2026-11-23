package menus

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// The editor's existing key/pointer consumers create and SAVE the mission. BEGIN
// reopens that document normally and selects the ordinary R5 launch default.
// No initial actor/control/result adjustment or new film adjustment is supplied.
func authoredOrdersController(t *testing.T) *FrontEnd {
	return authoredOrdersControllerProfile(t, replay.R5AuthoredCompatibility)
}

func authoredOrdersControllerProfile(t *testing.T, profile string) *FrontEnd {
	return authoredOrdersWithRuntime(t, profile, configuration.Defaults())
}
func authoredOrdersWithRuntime(t *testing.T, profile string, options configuration.RuntimeOptions) *FrontEnd {
	t.Helper()
	c := authoredUIController(t)
	c.Runtime = options
	doc := authorThroughUI(t, c)
	before, err := os.ReadFile(filepath.Join(c.MissionDirectory, doc.ID+".g2m"))
	if err != nil {
		t.Fatal(err)
	}
	c.Runtime.DisableEnemyAI, c.Runtime.DisableWingmenAI = true, false
	editorKey(t, c, "b")
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("saved mission BEGIN did not reach briefing", c.Screen, c.Message)
	}
	switches, err := gamestate.LaunchSwitches(c.Mission.Data, c.Mission.Settings, c.Office.Assets.Missions.Generation.Tables.Initialization.CPGMapping)
	if err != nil {
		t.Fatal(err)
	}
	c.enemyLaunchRules, err = freezePreparedEnemyRules(c.Office.Assets.Missions, c.Mission.Data, switches, c.Runtime.DisableEnemyAI)
	if err != nil {
		t.Fatal(err)
	}
	c.simulationProfile = profile // Explicit legacy/current policy control.
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Rendering == nil || !c.Rendering.r5Mechanics || c.Rendering.filmRecorder == nil || c.Mission.ResultLaunch == nil || len(c.Rendering.Initialized.FlightRows) != 5 {
		t.Fatal("ordinary five-aircraft R5/result/recorder launch missing", c.Mission.ResultUnavailable)
	}
	after, err := os.ReadFile(filepath.Join(c.MissionDirectory, doc.ID+".g2m"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("BEGIN changed saved document", err)
	}
	t.Cleanup(c.AbandonFilmRecording)
	return c
}

func runLogicalOrders(t *testing.T, c *FrontEnd) {
	t.Helper()
	for _, action := range []Actions{{}, {ViewKey: 5, ViewShift: true}, {ViewKey: 1}, {FlightCommands: []int{2}}} {
		if err := c.Handle(action); err != nil {
			t.Fatal(err)
		}
	}
	if c.Rendering.Flight.OrderRecipient != 1 {
		t.Fatal("Shift+F5 did not select actual member1")
	}
	for i := 0; i < 32; i++ {
		action := Actions{}
		switch i {
		case 3:
			action.WingmanOrder = "h"
		case 6:
			action.Jettison = true
		case 8, 10:
			action.LastMessage = true
		case 12:
			action.WingmanOrder = "b"
		}
		if err := c.Handle(action); err != nil {
			t.Fatal(err)
		}
		if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
			t.Fatal(err)
		}
		if c.Rendering == nil || c.Rendering.Flight.Failure != "" {
			t.Fatal("short logical order sequence failed")
		}
		if i == 3 && c.Rendering.Initialized.Entities[1].ControlRaw[1] != 2 {
			t.Fatal("actual selected wingman did not enter HOLD")
		}
		if i == 12 && c.Rendering.Initialized.Entities[1].ControlRaw[1] != 8 {
			t.Fatal("actual selected wingman did not enter RTB route")
		}
	}
}

func TestNormallySavedOrdersRecordingEqualityStrictAndResume(t *testing.T) {
	seed := authoredOrdersController(t)
	var launch filmLaunch
	if err := json.Unmarshal(seed.Rendering.filmRecorder.Film.Launch, &launch); err != nil {
		t.Fatal(err)
	}
	seed.AbandonFilmRecording()
	var want map[string]string
	for _, record := range []bool{true, false} {
		final, err := cloneFilmFinal(&launch.Final)
		if err != nil {
			t.Fatal(err)
		}
		c := &FrontEnd{Office: seed.Office, Runtime: launch.Runtime, Screen: ScreenScene,
			simulationProfile: replay.R5AuthoredCompatibility, enemyLaunchRules: launch.EnemyRules,
			filmIsolation: &filmSessionIsolation{Mode: "R5-recording-equivalence"},
			Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight,
				ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
		if err = c.constructSimulation(final); err != nil {
			t.Fatal(err)
		}
		if record {
			c.FilmDirectory = t.TempDir()
			c.filmIsolation = nil
			c.beginFilmRecording(final)
			c.filmIsolation = &filmSessionIsolation{Mode: "R5-recording-equivalence"}
			if c.Rendering.filmRecorder == nil {
				t.Fatal("optional recorder did not open")
			}
		}
		runLogicalOrders(t, c)
		checkpoint := c.Rendering.FilmCheckpoint()
		if want == nil {
			want = checkpoint
		} else if !reflect.DeepEqual(want, checkpoint) {
			t.Fatal("optional recording changed authoritative state")
		}
		c.AbandonFilmRecording()
	}

	ordinary := authoredOrdersController(t)
	runLogicalOrders(t, ordinary)
	for _, action := range []Actions{{}, {EndMission: true}, {}, {ConfirmQuit: true}} {
		if err := ordinary.Handle(action); err != nil {
			t.Fatal(err)
		}
	}
	if ordinary.Screen != ScreenDebrief {
		t.Fatal("normal end did not reach debrief", ordinary.Screen, ordinary.Message)
	}
	film := verifyFilm(t, ordinary)
	if film.Header.Simulation != replay.R5AuthoredCompatibility || !film.SafeGameplayMechanics() {
		t.Fatal("complete film lost frozen R5 interpretation", film.Header)
	}
	sortieID := ordinary.Debrief.Pending.Start.ID
	loopAcknowledge(t, ordinary)
	committed := filmJSON(ordinary.Store.State())
	if err := ordinary.Store.CommitSortie(sortieID); err != nil || !bytes.Equal(committed, filmJSON(ordinary.Store.State())) {
		t.Fatal("repeated acknowledgment changed committed result", err)
	}
	reopened, err := gamestate.OpenSaveStore(ordinary.Store.Directory, "")
	if err != nil || !bytes.Equal(committed, filmJSON(reopened.State())) {
		t.Fatal("result restart differs", err)
	}
	for _, scale := range []int{1, 2} {
		options := configuration.Defaults()
		options.DisableEnemyAI = !launch.Runtime.DisableEnemyAI
		options.DisableWingmenAI = !launch.Runtime.DisableWingmenAI
		options.SimulationRenderScale = scale
		view, err := NewFilmPlayback(seed.Office.Assets, film, options)
		if err != nil {
			t.Fatal(err)
		}
		for !view.Done {
			if err = view.Step(); err != nil {
				t.Fatal(err)
			}
		}
		view, err = NewFilmPlayback(seed.Office.Assets, film, options)
		if err != nil {
			t.Fatal(err)
		}
		for view.reachedSteps < 16 {
			if err = view.Step(); err != nil {
				t.Fatal(err)
			}
		}
		before := view.Preview().FilmCheckpoint()
		if err = view.Resume(); err != nil || !view.Practice || view.Engine.Store != nil || view.Engine.Mission.ResultLaunch != nil || !view.Preview().r5Mechanics {
			t.Fatal("isolated Resume lost policy/capability", err)
		}
		if before["combat"] != view.Preview().FilmCheckpoint()["combat"] || !bytes.Equal(committed, filmJSON(ordinary.Store.State())) {
			t.Fatal("Resume changed combat or real career")
		}
	}
	if out := os.Getenv("GS2000_R5_OUTPUT"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := replay.Marshal(film)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "r5-normal-authored-orders.g2r"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		receipt, _ := json.MarshalIndent(map[string]any{"scope": "Normal editor UI SAVE/load/BEGIN, original actor initialization, logical Hold/Recall and manual input; no runtime/film-initial adjustment or completion injected", "recording_equal": true, "strict_scales": []int{1, 2}, "preferences_reversed": true, "isolated_resume": true, "committed_once": true, "retask_film": false}, "", "  ")
		if err = os.WriteFile(filepath.Join(out, "r5-normal-authored-orders.json"), receipt, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuthoredExportProjectsRuntimeObjectiveLineage(t *testing.T) {
	c := authoredOrdersController(t)
	for i := 0; i < 2; i++ {
		if err := c.Handle(Actions{}); err != nil {
			t.Fatal(err)
		}
		if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	for _, action := range []Actions{{}, {EndMission: true}, {}, {ConfirmQuit: true}} {
		if err := c.Handle(action); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenDebrief {
		t.Fatal("ordinary end did not reach debrief", c.Screen, c.Message)
	}
	film, err := c.SealRecordedFilm()
	if err != nil || film == nil || !film.SafeGameplayMechanics() {
		t.Fatal("complete R5 film missing", err)
	}
	var launch filmLaunch
	if err = json.Unmarshal(film.Launch, &launch); err != nil || launch.AuthoredSource == nil {
		t.Fatal("saved source document not frozen", err)
	}
	originalPath := filepath.Join(c.MissionDirectory, launch.AuthoredSource.ID+".g2m")
	original, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	loopAcknowledge(t, c)
	// The shared acknowledgment helper also uses EXIT to consume the post-sortie
	// return token. Reopen the ordinary library before exercising its Export.
	if err = c.openFilmLibrary(false, "", ""); err != nil {
		t.Fatal(err)
	}
	if c.Library == nil || len(c.Library.Entries) != 1 {
		t.Fatal("normal film library admission missing")
	}
	filmPath := publishedFilmPath(t, c, film.Header.ID)
	filmBefore, err := os.ReadFile(filmPath)
	if err != nil {
		t.Fatal(err)
	}
	career := filmJSON(c.Store.State())
	viewer, err := NewFilmPlayback(c.Office.Assets, film, c.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Preview().ResultTargets.ObjectiveRevisions == nil {
		t.Fatal("regression did not reach R5's live objective owner")
	}
	initial := viewer.Preview().FilmCheckpoint()
	if err = c.exportFilmToMission(); err != nil {
		t.Fatal("runtime lineage blocked source-authoring comparison", err)
	}
	entries, err := (gamestate.AuthoredStore{Directory: c.MissionDirectory}).List()
	if err != nil || len(entries) != 2 {
		t.Fatal("editable export not saved", err)
	}
	for _, entry := range entries {
		if entry.ID == launch.AuthoredSource.ID {
			continue
		}
		exported, _, err := (gamestate.AuthoredStore{Directory: c.MissionDirectory}).Load(entry.ID)
		if err != nil || exported.Initial != nil || exported.Compiled.Targets.ObjectiveRevisions != nil || !reflect.DeepEqual(exported.Placements, launch.AuthoredSource.Placements) {
			t.Fatal("export lost editable source or retained runtime lineage", err)
		}
	}
	filmAfter, _ := os.ReadFile(filmPath)
	sourceAfter, _ := os.ReadFile(originalPath)
	if !bytes.Equal(filmBefore, filmAfter) || !bytes.Equal(original, sourceAfter) || !bytes.Equal(career, filmJSON(c.Store.State())) || !reflect.DeepEqual(initial, viewer.Preview().FilmCheckpoint()) {
		t.Fatal("export changed film, saved source, career or independent viewer")
	}
}
