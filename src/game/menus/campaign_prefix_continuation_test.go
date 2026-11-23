package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// This is a fresh launch consuming the retained normalized control prefix. It
// does not play/rescore the old film, replace checkpoints, inject a projectile,
// or open the personal runtime. The seven contact boundaries are separate tests.
func TestCampaignPrefixContinuation(t *testing.T) {
	a, old, launch, journal := retainedCampaign(t)
	c := missionController(t)
	// This retained continuation is the R2.2 interpretation control. New
	// default-launch rules are exercised by the R221 ordinary-launch test.
	c.simulationProfile = replay.R22Compatibility
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
	// An explicit historical interpretation supplies its frozen launch bank,
	// just as playback and the retained R22 natural-encounter fixture do.
	switches, err := gamestate.LaunchSwitches(session.Data, session.Settings, a.Missions.Generation.Tables.Initialization.CPGMapping)
	if err != nil {
		t.Fatal(err)
	}
	c.enemyLaunchRules, err = freezePreparedEnemyRules(a.Missions, session.Data, switches, c.Runtime.DisableEnemyAI)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.constructSimulation(final); err != nil {
		t.Fatal(err)
	}
	c.beginFilmRecording(final)
	p, r := c.Rendering, c.Rendering.Flight
	if !p.wingmanDamageMechanics || p.Initialized.Stores.NPCDamage == nil || p.filmRecorder == nil {
		t.Fatal("new launch owner missing")
	}
	beforePilot := filmJSON(c.Store.State().SelectedPilot())
	var trail []map[string]any
	var first map[string]any
	last := 0
	write := func(out map[string]any) {
		if dir := os.Getenv("GS2000_R22_OUTPUT"); dir != "" {
			if e := os.MkdirAll(dir, 0700); e != nil {
				t.Fatal(e)
			}
			data, e := json.MarshalIndent(out, "", "  ")
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(dir, "campaign-continuation.json"), append(data, '\n'), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	for i, op := range old.Operations {
		last = i
		if c.Screen == ScreenDebrief {
			break
		}
		// Reset only the bounded operational receipt buffer at the known
		// witness tick; it is excluded from authoritative state/checkpoints.
		if i == 3393 {
			r.CombatEvents = nil
		}
		beforeEvents := len(r.CombatEvents)
		beforeDamage := p.Initialized.Stores.Damage12(1)
		beforeFlares := p.Initialized.Stores.Ships[1].Flares
		beforeResults := r.Combat.FilmState().Results
		switch op.Kind {
		case "step", "terminal":
			var v filmStep
			if err = json.Unmarshal(op.Payload, &v); err != nil {
				t.Fatal(err)
			}
			// Old controls are an input prefix, not authority over the new clock
			// or RNG after the newly implemented branches start consuming draws.
			timing := r.Clock.ReplayTiming()
			if i <= 3393 && timing != v.Timing {
				t.Fatal("clock differs before first new branch", i)
			}
			r.filmTiming = &timing
			r.Paused, p.Paused, r.terminalAwaitingAck = false, false, false
			var commands []int
			if v.Input.HasCommand {
				commands = []int{v.Input.Command}
			}
			r.commandInput.samples = []flightInputSample{{at: 0, commands: commands, aux: v.Input.Aux, directions: v.Input.Directions, trigger: v.Input.Trigger, press: v.Input.Pressed}}
			err = c.advanceFlight(0)
		case "boundary":
			var v filmBoundary
			if err = json.Unmarshal(op.Payload, &v); err != nil {
				t.Fatal(err)
			}
			n := p.filmRecorder.operationCount()
			err = c.applyFilmBoundary(v)
			if err == nil && n == p.filmRecorder.operationCount() {
				c.recordFilmBoundary(v)
			}
		case "end":
			if p.Initialized.Sortie.Result == nil && !p.Initialized.AnswerNaturalEnd(true) {
				t.Fatal("new run has no reached natural ending", i)
			}
			err = c.finishSortieResult()
		}
		if i >= 3388 {
			trail = append(trail, map[string]any{"operation": i, "steps": r.Steps, "phase": r.ElapsedDA, "rng": p.Initialized.RandomState, "flares": p.Initialized.Stores.Ships[1].Flares, "damage": p.Initialized.Stores.Damage12(1), "kind": p.Initialized.Entities[1].Kind, "participation": p.Initialized.Entities[1].Participation})
			if len(trail) > 48 {
				trail = trail[len(trail)-48:]
			}
		}
		if i == 3393 {
			found := false
			for _, e := range r.CombatEvents[beforeEvents:] {
				if e.Kind == "hostile-defended" && e.Actor == 55 && e.Target == 1 && e.Value == 186 && strings.Contains(strings.ToLower(e.Source), "c5ba") {
					found = true
					first = map[string]any{"operation": i, "steps": r.Steps, "event": e, "damage_before": beforeDamage, "damage_after": p.Initialized.Stores.Damage12(1), "flares_before": beforeFlares, "flares_after": p.Initialized.Stores.Ships[1].Flares, "result_events_before": beforeResults, "result_events_after": r.Combat.FilmState().Results}
				}
			}
			if !found || beforeFlares != 30 || p.Initialized.Stores.Ships[1].Flares != 27 || beforeDamage != p.Initialized.Stores.Damage12(1) {
				t.Fatal("first exact defense differs", found, first, r.CombatEvents[beforeEvents:])
			}
			for _, e := range r.CombatEvents[beforeEvents:] {
				if e.Target == 1 && e.Kind == "damage" || e.Actor == 1 && e.Kind == "notification" && e.Notification == 4 {
					t.Fatal("defense invented damage/notice", e)
				}
			}
			for _, e := range r.Combat.FilmState().Results {
				if e.Sequence > beforeResults[len(beforeResults)-1].Sequence && e.Kind == 2 && e.Actor == 1 {
					t.Fatal("defense invented damage event", e)
				}
			}
		}
		if err != nil || r.Failure != "" || p.Error != nil {
			write(map[string]any{"class": "fresh-launch retained-input diagnostic; not original/new OS flight", "first_defense": first, "first_later_guard_operation": i, "error": fmt.Sprint(err), "failure": r.Failure, "preview_error": fmt.Sprint(p.Error), "steps": r.Steps, "phase": r.ElapsedDA, "world": p.Initialized.World, "rng": p.Initialized.RandomState, "trail": trail, "follower_limits": p.Initialized.FollowerLimits, "actor_mask": r.Systems.UnsupportedFollowerEmptyBurn, "actors": p.Initialized.Entities[:9], "stores": p.Initialized.Stores, "result_exists": p.Initialized.Sortie.Result != nil})
			t.Fatal("new Campaign continuation guard", i, err, r.Failure, p.Error)
		}
		p.takeSceneAudio()
	}
	if first == nil || c.Screen != ScreenDebrief || c.Debrief == nil {
		write(map[string]any{"first_defense": first, "last_operation": last, "screen": c.Screen, "trail": trail})
		t.Fatal("no complete new outcome", c.Screen, last)
	}
	if !bytes.Equal(beforePilot, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("premature career publication")
	}
	proposal := c.Debrief.Pending.Proposal
	film, err := c.SealRecordedFilm()
	if err != nil {
		t.Fatal(err)
	}
	if film.Header.Simulation != replay.R22Compatibility {
		t.Fatal(film.Header.Simulation)
	}
	pages := acknowledgeLibrary(t, c)
	once := c.Store.State()
	if err = c.Store.CommitSortie(proposal.Facts.SortieID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(once, c.Store.State()) {
		t.Fatal("duplicate result")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !reflect.DeepEqual(once, reopened.State()) {
		t.Fatal("restart", err)
	}
	play, err := NewFilmPlayback(a, film, c.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	for !play.Done {
		if err = play.Step(); err != nil {
			t.Fatal("strict", err)
		}
	}
	if play.Engine.Store != nil {
		t.Fatal("playback career authority")
	}
	write(map[string]any{"class": "fresh-launch retained-input diagnostic, complete actual outcome; not original/new OS flight", "first_defense": first, "last_operation": last, "world": p.Initialized.World, "actual_score": proposal.Score.Score, "proposal": proposal, "pages": pages, "strict_operations": play.Cursor, "exactly_once": true, "trail": trail})
	if dir := os.Getenv("GS2000_R22_OUTPUT"); dir != "" {
		data, e := replay.Marshal(film)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, "campaign-new-r22.g2r"), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Log("Exact first defense and actual Campaign outcome; strict", play.Cursor, "score", proposal.Score.Score)
}
