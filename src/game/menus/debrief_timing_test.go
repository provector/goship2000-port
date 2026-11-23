package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Native diagnostic, not an OS-input or long-film timing claim. The saved
// mission/preflight/ending/result/film owners are real; no completion flags or
// pending result are supplied. Measurements use monotonic wall time, never Tick.
func TestCrewPhotoFreshClickAndFilmStageTiming(t *testing.T) {
	c := naturalEncounterNaturalController(t, true)
	beforePilot := c.Store.State().SelectedPilot()
	act(t, c, Actions{})
	act(t, c, Actions{FlightCommands: []int{2}})
	jStep(t, c, 10)
	if c.Rendering.filmRecorder == nil || c.Rendering.filmRecorder.Spool == nil {
		t.Fatal("actual file-backed recording required")
	}
	start := time.Now()
	act(t, c, Actions{EndMission: true})
	endPrompt := time.Since(start)
	if c.Screen != ScreenQuit || !c.sceneEndConfirmation {
		t.Fatal("normal ending confirmation absent")
	}
	start = time.Now()
	act(t, c, Actions{ConfirmQuit: true})
	endingStage := time.Since(start) // Includes actual Spool.Seal, result staging and page setup.
	if c.Screen != ScreenDebrief || c.filmPending == nil || !c.filmPending.Header.Complete {
		t.Fatal("normal ending did not seal before debrief", c.Screen, c.filmError)
	}
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != beforePilot.Statistics.MissionCampaignCounter {
		t.Fatal("ending published before acknowledgment")
	}
	start = time.Now()
	recorded, err := c.SealRecordedFilm() // Read-only Validate of the already sealed film.
	validate := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	var latch inputLatch
	dispatch := func(b Buttons) {
		t.Helper()
		if err := c.Handle(latch.Read(b)); err != nil {
			t.Fatal(err)
		}
	}
	// A fresh press skips an unattended neighboring dwell, and a second press
	// advances. Hold the edge that enters CREWPRO across its first visible frame.
	for i := 0; i < 24 && c.Debrief.Page != "crew"; i++ {
		dispatch(Buttons{})
		dispatch(Buttons{LeftMouse: true})
	}
	if c.Debrief.Page != "crew" || c.Debrief.Tick != 0 || c.fade != nil {
		t.Fatal("crew was not immediately available without fade/dwell", c.Debrief.Page, c.Debrief.Tick, c.fade)
	}
	start = time.Now()
	frame := c.Frame(true)
	firstFrameRender := time.Since(start)
	if len(frame) != 64000 {
		t.Fatal("visible crew frame absent")
	}
	visible := time.Now()
	crew := c.Debrief
	selection := crew.CrewSelection
	dispatch(Buttons{LeftMouse: true}) // Still held from the entering page: no fresh edge.
	if c.Debrief != crew || crew.Page != "crew" || crew.CrewSelection != selection || crew.Tick != 0 {
		t.Fatal("entry edge dismissed or mutated crew page")
	}
	dispatch(Buttons{})
	dispatch(Buttons{MenuEnd: true}) // Source ACCEPT row, not a photograph/gap.
	dispatch(Buttons{})
	start = time.Now()
	dispatch(Buttons{Inside: true, X: 3, Y: 173, LeftMouse: true})
	clickDispatch := time.Since(start)
	visibleToAccepted := time.Since(visible)
	if c.Debrief != crew || crew.Page == "crew" || crew.Tick != 0 {
		t.Fatal("fresh ACCEPT did not work at Tick0", crew.Page, crew.Tick)
	}
	if c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != beforePilot.Statistics.MissionCampaignCounter {
		t.Fatal("crew ACCEPT committed before final feedback acknowledgment")
	}
	// This fresh, loss-free fixture proceeds to feedback. Its deliberate dwell
	// is skipped by one fresh click; that edge must not also commit the result.
	if crew.Page != "feedback" {
		t.Fatal("neighboring feedback control absent", crew.Page)
	}
	dispatch(Buttons{})
	dispatch(Buttons{LeftMouse: true})
	if c.Debrief != crew || crew.Tick != 120 || c.Store.State().SelectedPilot().Statistics.MissionCampaignCounter != beforePilot.Statistics.MissionCampaignCounter {
		t.Fatal("feedback skip also acknowledged")
	}
	dispatch(Buttons{LeftMouse: true})
	if c.Debrief != crew {
		t.Fatal("held click acknowledged feedback")
	}
	dispatch(Buttons{})
	start = time.Now()
	dispatch(Buttons{LeftMouse: true})
	finalAck := time.Since(start) // Validate, durable career commit, PublishLast and library admission.
	if c.Screen != ScreenFilms || c.Debrief != nil || len(c.Library.Entries) != 1 {
		t.Fatal("final acknowledgment/film admission", c.Screen, c.filmError)
	}
	afterPilot := c.Store.State().SelectedPilot()
	if afterPilot.Statistics.MissionCampaignCounter != beforePilot.Statistics.MissionCampaignCounter+1 {
		t.Fatal("wrong durable result count")
	}
	after, _ := json.Marshal(c.Store.State())
	for i := 0; i < 4; i++ {
		dispatch(Buttons{})
		dispatch(Buttons{LeftMouse: true})
	}
	if err = c.commitDebriefResult(); err != nil { // A delayed old callback is inert too.
		t.Fatal(err)
	}
	again, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(after, again) {
		t.Fatal("rapid clicks duplicated career publication")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, filepath.Join(t.TempDir(), "no-original"))
	if err != nil {
		t.Fatal(err)
	}
	restarted, _ := json.Marshal(reopened.State())
	if !bytes.Equal(after, restarted) {
		t.Fatal("restart changed committed result")
	}
	start = time.Now()
	err = (replay.Store{Root: t.TempDir()}).PublishLast(recorded)
	standalonePublish := time.Since(start) // Independent fresh destination; not a nested finalAck subspan.
	if err != nil {
		t.Fatal(err)
	}
	filmBytes, err := replay.Marshal(recorded)
	if err != nil {
		t.Fatal(err)
	}
	note := map[string]any{
		"simulation_profile": recorded.Header.Simulation,
		"class":              "Native/headless normally saved R22 GUN launch, short actual recorded simulation and normal Alt+E confirmation; synthetic window buttons, no physical/DOS claim",
		"operations":         len(recorded.Operations), "film_bytes": len(filmBytes),
		"end_prompt_ns": endPrompt.Nanoseconds(), "ending_seal_result_stage_ns": endingStage.Nanoseconds(),
		"sealed_film_readonly_validation_ns": validate.Nanoseconds(), "first_crew_frame_render_ns": firstFrameRender.Nanoseconds(),
		"first_visible_to_accepted_fresh_click_ns": visibleToAccepted.Nanoseconds(), "fresh_accept_dispatch_ns": clickDispatch.Nanoseconds(),
		"crew_tick_at_accept": 0, "crew_dwell_ticks_required": 0, "crew_fade_active": false,
		"entry_held_edge_consumed": true, "feedback_skip_did_not_commit": true,
		"final_ack_validate_commit_publish_library_ns": finalAck.Nanoseconds(),
		"standalone_publish_fresh_destination_ns":      standalonePublish.Nanoseconds(),
		"mission_count_increment":                      1, "restart_equal": true, "rapid_clicks_equal": true,
		"measurement_limit": "Ending stage includes actual spool sealing and result staging; no claim of an instrumented Seal-only subspan. Standalone Validate/Publish timings are independent controls, not an additive breakdown. Short film does not reproduce a long-film stall.",
	}
	t.Logf("crew readiness/timing: %+v", note)
	if out := os.Getenv("GS2000_R221_OUTPUT"); out != "" {
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(note, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "debrief-timing.json"), append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
