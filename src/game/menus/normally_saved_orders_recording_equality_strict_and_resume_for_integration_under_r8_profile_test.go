package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormallySavedOrdersRecordingEqualityStrictAndResumeForIntegrationUnderR8Profile(t *testing.T) {
	verifyOrdersRecordingAndResume(t, replay.R8AuthoredCompatibility, "GS2000_R8_OUTPUT", "r8-normal-authored-orders")
}
func TestCorrectedOrdersRecordingEqualityStrictAndResume(t *testing.T) {
	verifyOrdersRecordingAndResume(t, replay.R83AuthoredCompatibility, "GS2000_R83_OUTPUT", "r83-normal-authored-orders")
}
func verifyOrdersRecordingAndResume(t *testing.T, profile, outputKey, basename string) {
	t.Helper()
	// Exercise this archived policy even after the default advances.
	seed := authoredOrdersControllerProfile(t, profile)
	var launch filmLaunch
	if err := json.Unmarshal(seed.Rendering.filmRecorder.Film.Launch, &launch); err != nil {
		t.Fatal(err)
	}
	if !seed.Rendering.r8Mechanics || !seed.Rendering.r6Mechanics || !seed.Rendering.r5Mechanics || !seed.Rendering.r4Mechanics || !seed.Rendering.copilotMechanics || !seed.Rendering.Initialized.Stores.VictimWingStores || !seed.Rendering.Initialized.Stores.VictimCriticalProficiency {
		t.Fatal("R8 default lost inherited policy")
	}
	seed.AbandonFilmRecording()
	var want map[string]string
	for _, record := range []bool{true, false} {
		final, err := cloneFilmFinal(&launch.Final)
		if err != nil {
			t.Fatal(err)
		}
		c := &FrontEnd{Office: seed.Office, Runtime: launch.Runtime, Screen: ScreenScene,
			simulationProfile: profile, enemyLaunchRules: launch.EnemyRules,
			filmIsolation: &filmSessionIsolation{Mode: "R8-recording-equivalence"},
			Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight,
				ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
		if err = c.constructSimulation(final); err != nil {
			t.Fatal(err)
		}
		if record {
			c.FilmDirectory = t.TempDir()
			c.filmIsolation = nil
			c.beginFilmRecording(final)
			c.filmIsolation = &filmSessionIsolation{Mode: "R8-recording-equivalence"}
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

	ordinary := authoredOrdersControllerProfile(t, profile)
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
	if film.Header.Simulation != profile || !film.SafeGameplayMechanics() {
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
	// Acknowledgment assigns the final score to the published film. Reopen the
	// same file the player selects in the Film Library after a restart.
	published := replay.Store{Root: ordinary.FilmDirectory}
	film, err = published.LoadEntry(replay.Entry{ID: film.Header.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		options := configuration.Defaults()
		options.DisableEnemyAI = !launch.Runtime.DisableEnemyAI
		options.DisableWingmenAI = !launch.Runtime.DisableWingmenAI
		options.SimulationRenderScale = scale
		options.AvoidancePolicy = configuration.AvoidanceLegacy
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
		if (profile == replay.R83AuthoredCompatibility || profile == replay.R86AuthoredCompatibility || profile == replay.T5AuthoredCompatibility) && (!view.Preview().Flight.State.CorrectedAvoidance || !view.Preview().Flight.State.SupportSampleValid) {
			t.Fatal("practice lost recorded avoidance state")
		}
		before := view.Preview().FilmCheckpoint()
		if err = view.Resume(); err != nil || !view.Practice || view.Engine.Store != nil || view.Engine.Mission.ResultLaunch != nil || (!view.Preview().r5Mechanics || !view.Preview().r8Mechanics || !view.Preview().Initialized.Stores.VictimCriticalProficiency) {
			t.Fatal("isolated Resume lost policy/capability", err)
		}
		if before["combat"] != view.Preview().FilmCheckpoint()["combat"] || !bytes.Equal(committed, filmJSON(ordinary.Store.State())) {
			t.Fatal("Resume changed combat or real career")
		}
	}
	if out := os.Getenv(outputKey); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(publishedFilmPath(t, ordinary, film.Header.ID))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, basename+".g2r"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		receipt, _ := json.MarshalIndent(map[string]any{"scope": "Normal editor UI SAVE/load/BEGIN, original actor initialization, logical Hold/Recall and manual input; no runtime/film-initial adjustment or completion injected", "recording_equal": true, "strict_scales": []int{1, 2}, "preferences_reversed": true, "isolated_resume": true, "committed_once": true, "retask_film": false}, "", "  ")
		if err = os.WriteFile(filepath.Join(out, basename+".json"), receipt, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyChoiceFrozen(t *testing.T) {
	opts := configuration.Defaults()
	opts.AvoidancePolicy = configuration.AvoidanceLegacy
	c := authoredOrdersWithRuntime(t, "", opts)
	if c.Rendering.Flight.State.CorrectedAvoidance {
		t.Fatal("legacy choice overridden")
	}
	runLogicalOrders(t, c)
	for _, a := range []Actions{{}, {EndMission: true}, {}, {ConfirmQuit: true}} {
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	film := verifyFilm(t, c)
	var launch filmLaunch
	if err := json.Unmarshal(film.Launch, &launch); err != nil || launch.Runtime.AvoidancePolicy != configuration.AvoidanceLegacy || !film.R83Mechanics() {
		t.Fatal("recorded Legacy policy absent", err)
	}
	v, err := NewFilmPlayback(c.Office.Assets, film, configuration.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if v.Preview().Flight.State.CorrectedAvoidance {
		t.Fatal("current preference reinterpreted film")
	}
	if out := os.Getenv("GS2000_R83_OUTPUT"); out != "" {
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := replay.Marshal(film)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "r83-legacy.g2r"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOrdersRecordingEqualityStrictAndResume(t *testing.T) {
	verifyOrdersRecordingAndResume(t, replay.R86AuthoredCompatibility, "GS2000_R86_OUTPUT", "r86-authored-orders")
}

func TestOrdersRecordingEqualityStrictAndResumeForIntegration(t *testing.T) {
	verifyOrdersRecordingAndResume(t, replay.T5AuthoredCompatibility, "GS2000_T5_OUTPUT", "t5-authored-orders")
}
