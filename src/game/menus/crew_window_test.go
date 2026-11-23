//go:build !headless

package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The initial near-pickup/loaded-return poses are labelled native supplied state.
// Actual preparation, pickup, return and result producers run before real OS input.
func TestConnectedCrewPointer(t *testing.T) {
	out := os.Getenv("GS2000_N5_CREW_NATIVE")
	if out == "" {
		t.Skip("fresh connected output directory required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(filepath.Join(out, "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	pilot, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	pilot.Name = "N5 Crew Input"
	st := store.State()
	st.Slots[0], st.SelectedPilotID = pilot, pilot.ID
	if err = store.Commit(st); err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	options := configuration.Defaults()
	c.Runtime = options
	meta, err := c.ConfigureDevelopmentScenario("crew-choice-250")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err = c.AdvanceRendering(.12); err != nil {
		t.Fatal(err)
	}
	if c.Rendering.Paused {
		if err = c.Handle(Actions{Cancel: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err = c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if err = c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60 && c.Screen == ScreenScene; i++ {
		if err = c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen == ScreenQuit {
		if err = c.Handle(Actions{ConfirmQuit: true}); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenDebrief {
		t.Fatal("pickup/actual-return path did not reach result", c.Screen, c.Message)
	}
	for i := 0; i < 10 && c.Debrief.Page != "crew"; i++ {
		c.Debrief.Tick = 400 // Presentation-only semantic skip, not a result edit.
		if err = c.Handle(Actions{Enter: true}); err != nil {
			t.Fatal(err)
		}
		if err = c.Handle(Actions{}); err != nil {
			t.Fatal(err)
		}
	}
	if c.Debrief == nil || c.Debrief.Page != "crew" || len(c.Debrief.Pending.Proposal.Result.Recommendations) == 0 {
		t.Fatal("genuine crew recommendation missing")
	}
	initial := c.Debrief.Pending
	if initial.Proposal.Score.Score != 250 {
		t.Fatal("source five-Blackhawk contribution regressed", initial.Proposal.Score)
	}
	write := func(name string, value any) {
		t.Helper()
		raw, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("fixture.json", map[string]any{"scenario": meta, "scratch_save": store.Directory, "initial_pending": initial, "scope": "Real native OS input starts after genuine native preparation/pickup/staged-return/result. No natural original or continuously flown sortie claim."})
	if err = RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, DrawPointer: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 4200, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit || c.previousScreen != ScreenOffice {
		t.Fatal("helper did not acknowledge crew/result and quit from office", c.Screen, c.previousScreen)
	}
	final, err := store.PendingSortie()
	if err != nil {
		t.Fatal(err)
	}
	if final != nil {
		t.Fatal("result remained pending after OS acknowledgement")
	}
	committed := store.State().SelectedPilot()
	before := initial.Proposal.PilotBefore
	changed := 0
	promotedSourceIndex := -1
	for i, row := range committed.SquadronPilots {
		if row.Rank != before.SquadronPilots[i].Rank {
			changed++
			promotedSourceIndex = i + 1
		}
	}
	if changed != 1 {
		t.Fatal("OS recipient cycle/accept did not change exactly one stored rank", changed)
	}
	if promotedSourceIndex == initial.Proposal.Result.Recommendations[0].SourceIndex {
		t.Fatal("OS input accepted only the default recipient instead of reassigning")
	}
	reopened, err := gamestate.OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.State(), reopened.State()) {
		t.Fatal("commit/restart differed")
	}
	write("native-summary.json", map[string]any{"checks": map[string]bool{"genuine_pickup_return": true, "real_window_exit": true, "crew_result_commit": true, "exactly_one_crew_rank_change": true, "restart_identical": true}, "initial_pending": initial, "committed_pilot": committed, "scope": "Fixture assertions plus real OS input; host helper must separately prove mouse and keyboard recipient/gap/Accept events from immutable pending choices."})
}

// A real accepted-hit/loss/result produces the backup before the native window
// opens. The second Active pilot is an explicitly supplied roster precondition.
func TestConnectedInactiveRecovery(t *testing.T) {
	out := os.Getenv("GS2000_N5_ROSTER_NATIVE")
	if out == "" {
		t.Skip("fresh connected output directory required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(filepath.Join(out, "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	pilot, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	pilot.Name = "N5 Recovery Pilot"
	s := store.State()
	s.Slots[0], s.SelectedPilotID = pilot, pilot.ID
	if err = store.Commit(s); err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	options := configuration.Defaults()
	c.Runtime = options
	meta, err := c.ConfigureDevelopmentScenario("critical-fuel-hit")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60 && c.Screen == ScreenScene; i++ {
		if err = c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenDebrief {
		t.Fatal("actual accepted fuel hits did not reach result", c.Screen)
	}
	loopAcknowledge(t, c)
	id := store.State().SelectedPilotID
	if store.State().SelectedPilot().Status != gamestate.StatusKIA {
		t.Fatal("scenario did not actually normalize KIA")
	}
	other, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	other.Name = "N5 Other Pilot"
	other.Squadron = "Other Squadron"
	s = store.State()
	s.Slots[1] = other
	if err = store.Commit(s); err != nil {
		t.Fatal(err)
	}
	otherBefore := store.State().Slots[1]
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	c.Screen, c.Highlight = ScreenRoster, 0
	write := func(name string, value any) {
		t.Helper()
		raw, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("fixture.json", map[string]any{"scenario": meta, "scratch_save": store.Directory, "recovery_pilot_id": id, "other_pilot_id": other.ID, "scope": "Native real prepared critical-hit result and commit; second Active supplied. Real OS input starts at roster with inactive row0/Active row1."})
	if err = RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, DrawPointer: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 4200, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit || c.previousScreen != ScreenOffice {
		t.Fatal("native helper did not finish recovery and quit", c.Screen, c.previousScreen)
	}
	s = store.State()
	if s.Slots[0].ID != id || s.Slots[0].Status != gamestate.StatusActive || s.Slots[0].Statistics.PhoenixCounter != -1 || s.SelectedPilotID != other.ID {
		t.Fatal("wrong restored/posted identity", s)
	}
	if !reflect.DeepEqual(s.Slots[1], otherBefore) {
		t.Fatal("unrelated pilot changed")
	}
	if rec, err := store.RecoveryCandidateForPilot(id); err != nil || rec != nil {
		t.Fatal("Phoenix not consumed once", err)
	}
	reopened, err := gamestate.OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, reopened.State()) {
		t.Fatal("restart differs")
	}
	write("native-summary.json", map[string]any{"checks": map[string]bool{"actual_prepared_loss_committed": true, "real_window_exit": true, "matching_target_restored": true, "other_pilot_posted_unchanged": true, "phoenix_consumed_once": true, "restart_identical": true}, "recovery_pilot": s.Slots[0], "scope": "Window input must post Active row1, inspect inactive row0, recover matching backup and quit; no original runtime claim."})
}
