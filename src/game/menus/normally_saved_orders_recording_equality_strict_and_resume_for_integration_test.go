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

func TestNormallySavedOrdersRecordingEqualityStrictAndResumeForIntegration(t *testing.T) {
	seed := authoredOrdersControllerProfile(t, replay.R6AuthoredCompatibility)
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
			simulationProfile: replay.R6AuthoredCompatibility, enemyLaunchRules: launch.EnemyRules,
			filmIsolation: &filmSessionIsolation{Mode: "R6-recording-equivalence"},
			Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight,
				ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
		if err = c.constructSimulation(final); err != nil {
			t.Fatal(err)
		}
		if record {
			c.FilmDirectory = t.TempDir()
			c.filmIsolation = nil
			c.beginFilmRecording(final)
			c.filmIsolation = &filmSessionIsolation{Mode: "R6-recording-equivalence"}
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

	ordinary := authoredOrdersControllerProfile(t, replay.R6AuthoredCompatibility)
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
	if film.Header.Simulation != replay.R6AuthoredCompatibility || !film.SafeGameplayMechanics() {
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
	if out := os.Getenv("GS2000_R6_OUTPUT"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := replay.Marshal(film)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "r6-normal-authored-orders.g2r"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		receipt, _ := json.MarshalIndent(map[string]any{"scope": "Normal editor UI SAVE/load/BEGIN, original actor initialization, logical Hold/Recall and manual input; no runtime/film-initial adjustment or completion injected", "recording_equal": true, "strict_scales": []int{1, 2}, "preferences_reversed": true, "isolated_resume": true, "committed_once": true, "retask_film": false}, "", "  ")
		if err = os.WriteFile(filepath.Join(out, "r6-normal-authored-orders.json"), receipt, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuthoredExportProjectsRuntimeObjectiveLineageForIntegration(t *testing.T) {
	c := authoredOrdersControllerProfile(t, replay.R6AuthoredCompatibility)
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
	originalPath := savedMissionPath(t, c.MissionDirectory, launch.AuthoredSource.ID)
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
