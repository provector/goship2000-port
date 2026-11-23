package menus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// Use a fresh P.3 launch and reached user-recorded controls, then choose a new
// real voluntary ending after both goals. No old checkpoint/pose/result is used.
func TestCampaignGoalBoundaryVoluntarySuccess(t *testing.T) {
	a, old, launch, journal := retainedCampaign(t)
	c := missionController(t)
	// Preserve this retained no-hostile control schedule's accepted rules;
	// its purpose is goal/result continuation, not a new enemy encounter.
	c.simulationProfile = replay.R12Compatibility
	c.Office.Assets, c.Menus, c.Runtime = a, a.Menus, launch.Runtime
	saved := c.Store.State()
	pilot := journal.Start.PilotBefore
	saved.Slots[0], saved.SelectedPilotID = pilot, pilot.ID
	if err := c.Store.Commit(saved); err != nil {
		t.Fatal(err)
	}
	session, err := prepareMissionSession(a.Missions, pilot, launch.Final.Mission.Inputs.Seed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	final, err := cloneFilmFinal(&launch.Final)
	if err != nil {
		t.Fatal(err)
	}
	final.Mission, final.Flight, final.Settings = session.Data, session.Flight, session.Settings
	session.Final, session.ReadinessRows = final, final.Aircraft
	c.Mission, c.Screen = session, ScreenScene
	if err = c.constructSimulation(final); err != nil {
		t.Fatal(err)
	}
	// This control specifically requires an active post-sortie successor. Keep
	// the source CRT result stream distinct from the shared flight LFSR: seed1
	// gives first raw41/value0, below this reached ending's survival threshold60.
	// A hash-of-new-sortie-ID seed can legitimately yield MIA and office instead.
	resultSeed := uint32(1)
	c.Mission.ResultLaunch.DevelopmentSeed = &resultSeed
	c.beginFilmRecording(final)
	p, r := c.Rendering, c.Rendering.Flight
	if p.filmRecorder == nil {
		t.Fatal("missing recorder", c.Message)
	}
	var timeline []map[string]any
	oldGoals := [2]bool{}
	oldKinds := [5]byte{}
	for i := range oldKinds {
		oldKinds[i] = p.Initialized.Entities[i].Kind
	}
	lastOp := -1
	for i, operation := range old.Operations {
		switch operation.Kind {
		case "step", "terminal":
			var step filmStep
			if err := json.Unmarshal(operation.Payload, &step); err != nil {
				t.Fatal(err)
			}
			timing := r.Clock.ReplayTiming()
			if timing != step.Timing {
				t.Fatal("recomputed clock", i)
			}
			r.filmTiming = &timing
			r.Paused, p.Paused, r.terminalAwaitingAck = false, false, false
			var commands []int
			if step.Input.HasCommand {
				commands = []int{step.Input.Command}
			}
			r.commandInput.samples = []flightInputSample{{at: 0, commands: commands, aux: step.Input.Aux, directions: step.Input.Directions, trigger: step.Input.Trigger, press: step.Input.Pressed}}
			if err := c.advanceFlight(0); err != nil {
				t.Fatal(i, err)
			}
		case "boundary":
			var boundary filmBoundary
			if err := json.Unmarshal(operation.Payload, &boundary); err != nil {
				t.Fatal(err)
			}
			n := p.filmRecorder.operationCount()
			if err := c.applyFilmBoundary(boundary); err != nil {
				t.Fatal(i, err)
			}
			if n == p.filmRecorder.operationCount() {
				c.recordFilmBoundary(boundary)
			}
		case "end":
			break
		}
		goals := [2]bool{r.Combat.Primary.Achieved, r.Combat.Secondary.Achieved}
		kinds := [5]byte{}
		for j := range kinds {
			kinds[j] = p.Initialized.Entities[j].Kind
		}
		if goals != oldGoals || kinds != oldKinds {
			timeline = append(timeline, map[string]any{"operation": i, "step": r.Steps, "goals": goals, "kinds": kinds, "fuel": append([]int16(nil), r.Systems.Fuel...)})
			oldGoals, oldKinds = goals, kinds
		}
		if p.filmRecorder.Error != "" {
			t.Fatal(i, p.filmRecorder.Error)
		}
		p.takeSceneAudio()
		if goals == [2]bool{true, true} && p.GPS == nil && c.Screen == ScreenScene {
			lastOp = i
			if err := c.Handle(Actions{EndMission: true}); err != nil {
				t.Fatal(err)
			}
			if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if c.Screen != ScreenDebrief {
		campaignEvidence(t, "goal-boundary-timeline", timeline)
		t.Fatal("no admitted both-goal voluntary ending", lastOp, c.Screen, c.Message, timeline)
	}
	proposal := c.Debrief.Pending.Proposal
	campaignEvidence(t, "goal-boundary", map[string]any{"qualification": "Fresh P.3 mission with actual retained normalized controls; new voluntary termination at first both-goal nonmodal boundary. Explicit separate result CRT seed1 controls the active-successor branch; simulation LFSR unchanged. Not continuous OS input.", "result_crt_seed": resultSeed, "last_original_operation": lastOp, "steps": r.Steps, "timeline": timeline, "proposal": proposal})
	if !proposal.Success || proposal.Result.Campaign == nil || proposal.Result.Campaign.Campaign.ResultCounters != [2]int{1, 0} {
		t.Fatal("genuine early ending success boundary", proposal.Score, timeline)
	}
	if proposal.AcceptedPilot.Status != gamestate.StatusActive || proposal.Result.Threshold != 60 || len(proposal.Result.RNG.Draws) == 0 || proposal.Result.RNG.Draws[0].Site != "player-survival" || proposal.Result.RNG.Draws[0].Raw15 != 41 || proposal.Result.RNG.Draws[0].Value != 0 {
		t.Fatal("controlled source survival branch changed", proposal.Result, proposal.AcceptedPilot.Status)
	}
	// Independently retain the opposite legitimate result branch from these same
	// reached facts. Seed10000's first source CRT raw32694 scales to99>60; goals
	// and score still succeed, but MIA cannot continue an active Campaign.
	encodedFacts, err := json.Marshal(proposal.Facts)
	if err != nil {
		t.Fatal(err)
	}
	var lossFacts gamestate.SortieFacts
	if err = json.Unmarshal(encodedFacts, &lossFacts); err != nil {
		t.Fatal(err)
	}
	lossFacts.ResultContext.Seed = 10000
	loss, err := gamestate.ProposeSortie(proposal.PilotBefore, lossFacts, c.Mission.ResultLaunch.Tables)
	if err != nil || loss.AcceptedPilot.Status != gamestate.StatusMIA || !loss.Success || loss.Score.Score != proposal.Score.Score || loss.Score.NormalizedFlags&8 == 0 || loss.Result.RNG.Draws[0].Raw15 != 32694 || loss.Result.RNG.Draws[0].Value != 99 {
		t.Fatal("source MIA branch was suppressed or simulation/result domains mixed", err, loss)
	}
	filmID := p.filmRecorder.Film.Header.ID
	pages := acknowledgeLibrary(t, c)
	committed := c.Store.State()
	film, err := (replay.Store{Root: c.FilmDirectory}).Load(filmID)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.activateFilmControl(15); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenReplay || c.Viewer == nil || c.Viewer.Playback.Engine.Store != nil {
		t.Fatal("Library PLAY lost session isolation", c.Screen)
	}
	playback := c.Viewer.Playback
	for !playback.Done {
		if err = playback.Step(); err != nil {
			t.Fatal("new full strict playback", err)
		}
	}
	if err = c.AdvanceReplay(0); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenFilms || c.Library == nil || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("playback return changed committed career or return token", c.Screen)
	}
	if dir := os.Getenv("GS2000_P3_OUTPUT"); dir != "" {
		data, err := replay.Marshal(film)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "goal-boundary-success.g2r"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A declared new successor seed is independent of the historical witness.
	// No old result/cache is changed to make preparation succeed.
	nextSeed := int64(38)
	c.MissionSeed = &nextSeed
	if err = c.exitFilmLibrary(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenInterstitial || c.Mission == nil {
		t.Fatal("supported successor did not prepare next mission", c.Screen, c.Message)
	}
	next := c.Mission
	for i := 0; i < 3; i++ {
		if err = c.exitFilmLibrary(); err != nil {
			t.Fatal(err)
		}
		if err = c.commitDebriefResult(); err != nil {
			t.Fatal(err)
		}
		if err = c.Store.CommitSortie(proposal.Facts.SortieID); err != nil {
			t.Fatal(err)
		}
		c.Frame(false)
	}
	if c.Mission != next || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("repeated EXIT or obsolete result regenerated successor/awards")
	}
	for i := 0; i < 100 && c.Screen == ScreenInterstitial; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("actual successor briefing not reached", c.Screen, c.Message)
	}
	if err = c.Handle(Actions{Quit: true}); err != nil {
		t.Fatal(err)
	}
	if err = c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("quitting uncommitted successor/restart changed prior result", err)
	}
	startup, err := NewFrontEnd(a, reopened)
	if err != nil || startup.Screen != ScreenOffice || startup.Mission != nil {
		t.Fatal("restart forced unfinished successor", err)
	}
	campaignEvidence(t, "goal-boundary-successor", map[string]any{"qualification": "One durable result, actual library PLAY/return, declared new successor seed38, actual next briefing, quit before acceptance and restart; no duplicate commit.", "next_seed": nextSeed, "next_world": next.Data.World, "next_scenario": next.Data.Scenario, "before": committed, "restarted": reopened.State()})
	t.Logf("actual first both-goal ending operation%d step%d score%d pages%v film%s", lastOp, r.Steps, proposal.Score.Score, pages, filmID)
}
