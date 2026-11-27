package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

func careerRetained(t *testing.T) (*assetdata.OfficeAssets, *replay.Film, filmLaunch) {
	t.Helper()
	path := os.Getenv("GS2000_P2_RETAINED_FILM")
	if path == "" {
		t.Skip("explicit protected P2 film required")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if assetdata.Hash(raw) != "76ed390f3473333fe05d59738b9e7be996d5472ae02d7ef5e15a2ffdeb7c3410" {
		t.Fatal("wrong protected film")
	}
	f, e := convertedHistoricalFilm(t, path)
	if e != nil {
		t.Fatal(e)
	}
	a, e := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	var launch filmLaunch
	if e = json.Unmarshal(f.Launch, &launch); e != nil {
		t.Fatal(e)
	}
	return a, f, launch
}

func TestProtectedFailedFilmStrict(t *testing.T) {
	a, f, l := careerRetained(t)
	if len(f.Operations) != 13953 || f.End.Steps != 9622 || f.End.Score != 0 || len(f.End.Checkpoint) != 10 {
		t.Fatal("protected extent changed")
	}
	for _, row := range l.Final.Flight[1:] {
		if !row.Incomplete {
			t.Fatal("historical flags changed")
		}
	}
	q, e := NewFilmPlayback(a, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if q.Engine.Store != nil || q.EventCount == 0 || q.Cursor != 13953 {
		t.Fatal("strict playback missing isolation/operations")
	}
	if _, e = q.Preview().Initialized.Stores.SourceShipRows(); e == nil {
		t.Fatal("historical unsupported proficiency became guessed")
	}
	t.Logf("13953 operations/9622 steps/all10 checkpoint domains/events/end strict; frozen score0; events%d", q.EventCount)
}

// A new generated launch consumes the recorded generator operands and preflight
// choices. Each logical command/boundary is then submitted to the normal runtime
// and recorded afresh; no recorded checkpoint, goal, outcome or pose is assigned.
func TestNewGeneratedWOCRecordedPickupResult(t *testing.T) {
	// This 567-point source comparison was established before P5 enabled the
	// ordinary aircraft/shared-fire consumers. Keep its exact interpretation.
	generatedWOCRecordedPickupResult(t, replay.P4Compatibility, 567)
}

func TestGeneratedWOCRecordedPickupOwnInterpretation(t *testing.T) {
	// The same real controls now leave the25-point Gazelle alive. This is a
	// new launch and new film, not a rescore of the preserved P1 witness.
	generatedWOCRecordedPickupResult(t, replay.P5Compatibility, 522)
}

func generatedWOCRecordedPickupResult(t *testing.T, profile string, expectedScore int32) {
	t.Helper()
	a, old, l := careerRetained(t)
	archive := os.Getenv("GS2000_P2_ABANDONED")
	if archive == "" {
		t.Fatal("explicit protected abandoned journal required")
	}
	raw, e := os.ReadFile(archive)
	if e != nil {
		t.Fatal(e)
	}
	var pending gamestate.PendingSortie
	if e = json.Unmarshal(raw, &pending); e != nil {
		t.Fatal(e)
	}
	pilot := pending.Start.PilotBefore
	if pilot == nil {
		t.Fatal("preflight pilot absent")
	}
	c := missionController(t)
	c.simulationProfile = profile
	c.Office.Assets, c.Menus, c.Runtime = a, a.Menus, l.Runtime
	saved := c.Store.State()
	saved.Slots[0] = pilot
	saved.SelectedPilotID = pilot.ID
	if e = c.Store.Commit(saved); e != nil {
		t.Fatal(e)
	}
	in := l.Final.Mission.Inputs
	if in.Seed != 1793807445713739578 || in.PreviousScenario != 1 || in.DrawStream != nil {
		t.Fatal("actual generator inputs changed")
	}
	s, e := prepareMissionSession(a.Missions, pilot, in.Seed, in.DrawStream, in.RetainedObject)
	if e != nil {
		t.Fatal(e)
	}
	// The retained film predates forward capture. Compare every serialized
	// mission field and check the new, non-simulation index separately.
	generated, err := json.Marshal(s.Data)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := json.Marshal(l.Final.Mission)
	if err != nil {
		t.Fatal(err)
	}
	if s.Data.GenerationIndex() == nil || l.Final.Mission.GenerationIndex() != nil {
		t.Fatal("generator-only editor provenance boundary")
	}
	if !bytes.Equal(generated, retained) {
		t.Fatal("complete generated mission differs")
	}
	final, e := cloneFilmFinal(&l.Final)
	if e != nil {
		t.Fatal(e)
	}
	final.Mission, final.Flight, final.Settings = s.Data, s.Flight, s.Settings
	s.Final = final
	s.ReadinessRows = final.Aircraft
	c.Mission = s
	if e = c.constructSimulation(final); e != nil {
		t.Fatal(e)
	}
	c.beginFilmRecording(final)
	p := c.Rendering
	r := p.Flight
	if p.filmRecorder == nil || s.ResultLaunch == nil {
		t.Fatal("new ordinary launch absent", s.ResultUnavailable)
	}
	for i, row := range final.Flight {
		want := byte(25)
		if i == 0 {
			want = 0
		}
		if row.Incomplete || !p.Initialized.CrewPerformance[i].SkillKnown || p.Initialized.Stores.Ships[i].Proficiency == nil || *p.Initialized.Stores.Ships[i].Proficiency != want {
			t.Fatal("new proficiency absent", i)
		}
	}
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	healthyFollowerLaunch := false
	var gazelleDamage []int16
	for i, op := range old.Operations {
		switch op.Kind {
		case "step", "terminal":
			var v filmStep
			if e = json.Unmarshal(op.Payload, &v); e != nil {
				t.Fatal(e)
			}
			timing := r.Clock.ReplayTiming()
			if timing != v.Timing {
				t.Fatal("clock differs", i)
			}
			r.filmTiming = &timing
			r.Paused, p.Paused = false, false
			r.terminalAwaitingAck = false
			// Use the normal input queue; only retained logical inputs enter it.
			var commands []int
			if v.Input.HasCommand {
				commands = []int{v.Input.Command}
			}
			r.commandInput.samples = []flightInputSample{{at: 0, commands: commands, aux: v.Input.Aux, directions: v.Input.Directions, trigger: v.Input.Trigger, press: v.Input.Pressed}}
			if e = c.advanceFlight(0); e != nil {
				t.Fatal(i, e)
			}
			for _, event := range r.CombatEvents {
				healthyFollowerLaunch = healthyFollowerLaunch || event.Kind == "launch" && event.Source == "A7E6:AF21 healthy follower"
				if event.Kind == "damage" && event.Target == 44 {
					gazelleDamage = append(gazelleDamage, event.Value)
				}
			}
			// This bounded diagnostic history is not a simulation/result owner.
			// Drain receipts once, as the actual window audio/event consumer does.
			r.CombatEvents = nil
		case "boundary":
			var b filmBoundary
			if e = json.Unmarshal(op.Payload, &b); e != nil {
				t.Fatal(e)
			}
			n := p.filmRecorder.operationCount()
			if e = c.applyFilmBoundary(b); e != nil {
				t.Fatal(i, e)
			}
			if p.filmRecorder.operationCount() == n {
				c.recordFilmBoundary(b)
			}
		case "end":
			if p.Initialized.Sortie.Result == nil {
				if !p.Initialized.AnswerNaturalEnd(true) {
					t.Fatal("natural base ending not pending")
				}
			}
			if e = c.finishSortieResult(); e != nil {
				t.Fatal(e)
			}
		default:
			t.Fatal(op.Kind)
		}
		if p.filmRecorder.Error != "" {
			t.Fatal(i, p.filmRecorder.Error)
		}
		p.takeSceneAudio()
	}
	if c.Screen != ScreenDebrief || c.Debrief == nil {
		t.Fatal("repaired source result missing", c.Screen, c.Message)
	}
	proposal := c.Debrief.Pending.Proposal
	if proposal.Score.Score != expectedScore || proposal.Score.NormalizedFlags != 129 {
		t.Fatalf("actual generated result %+v", proposal.Score)
	}
	if value := int16(binary.LittleEndian.Uint16(p.Initialized.Entities[44].Object.Raw[27:])); value != 25 || proposal.Score.AircraftContribution != 150 {
		t.Fatal("independent source object/aircraft operands changed", value, proposal.Score)
	}
	wantRaw, wantKind := int16(65), byte(0x1c)
	if profile == replay.P5Compatibility {
		wantRaw, wantKind = 40, 0
	}
	if proposal.Score.RawScore != wantRaw || p.Initialized.Entities[44].Kind != wantKind || proposal.Score.PreScale != int32(wantRaw)+100+150 {
		t.Fatal("profile-specific destruction/credit differs", profile, p.Initialized.Entities[44].Kind, proposal.Score)
	}
	wantDamage := []int16{12, 8, 10, 10}
	if profile == replay.P5Compatibility {
		wantDamage = []int16{8, 8, 10, 10}
	}
	if !reflect.DeepEqual(gazelleDamage, wantDamage) || healthyFollowerLaunch != (profile == replay.P5Compatibility) {
		t.Fatal("source-shared RNG/healthy fire discriminator", profile, healthyFollowerLaunch, gazelleDamage)
	}
	after, _ := json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(before, after) {
		t.Fatal("career changed before finalack")
	}
	found := false
	for _, cue := range c.audioCues {
		found = found || cue == 0x1b
	}
	if !found {
		t.Fatal("matched success cue1b absent", c.audioCues)
	}
	fresh, e := c.SealRecordedFilm()
	if e != nil || fresh == nil {
		t.Fatal("new recording failed", e)
	}
	if fresh.Header.ID == old.Header.ID {
		t.Fatal("new film reused historical identity")
	}
	encoded, e := replay.Marshal(fresh)
	if e != nil {
		t.Fatal(e)
	}
	fresh, e = replay.Unmarshal(encoded)
	if e != nil {
		t.Fatal(e)
	}
	resultID := c.Debrief.Pending.Start.ID
	pages := loopAcknowledge(t, c)
	if !careerContains(pages, "jump") || !careerContains(pages, "crew") || !careerContains(pages, "feedback") {
		t.Fatal("source pages missing", pages)
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(c.Store.State(), reopened.State()) {
		t.Fatal("restart changed committed career")
	}
	backup, e := reopened.RecoveryCandidate()
	if e != nil || backup == nil || !bytes.Equal(before, filmJSON(backup.State.SelectedPilot())) {
		t.Fatal("Phoenix is not independent pre-result pilot", e)
	}
	once := reopened.State()
	if e = reopened.CommitSortie(resultID); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(once, reopened.State()) {
		t.Fatal("duplicate result changed career")
	}
	published, e := (replay.Store{Root: c.FilmDirectory}).Load(fresh.Header.ID)
	if e != nil {
		t.Fatal("Last Mission not published after durable commit", e)
	}
	if published.End.Score != int16(expectedScore) || published.End.Summary == nil || published.End.Summary.Score != int16(expectedScore) {
		t.Fatal("new published score/summary", published.End)
	}
	if !reflect.DeepEqual(fresh.Operations, published.Operations) || !bytes.Equal(fresh.Launch, published.Launch) {
		t.Fatal("publication rewrote recorded initialization/operations")
	}
	fresh = published
	encoded, e = replay.Marshal(fresh)
	if e != nil {
		t.Fatal(e)
	}
	q, e := NewFilmPlayback(a, fresh, c.Runtime)
	if e != nil {
		t.Fatal(e)
	}
	for !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal("new recording strict replay", e)
		}
	}
	outputKey := "GS2000_P2_CAREER_OUTPUT"
	if profile == replay.P5Compatibility {
		outputKey = "GS2000_P5_CAREER_OUTPUT"
	}
	if out := os.Getenv(outputKey); out != "" {
		if e = os.WriteFile(filepath.Join(out, "new-generated-film.g2r"), encoded, 0600); e != nil {
			t.Fatal(e)
		}
		report := map[string]any{"qualification": "Fresh generation and normal runtime consumption of retained logical inputs; no new OS input or listening claim", "protected_input_sha256": "eba66fbab5ab0439cfd735bf44fd2f84c5c84e0b5384e700f1d9215d8c98b3fc", "new_film_sha256": assetdata.Hash(encoded), "new_film_header": fresh.Header, "new_film_end": fresh.End, "new_film_operations": len(fresh.Operations), "pages": pages, "proposal": proposal, "crew_performance": p.Initialized.CrewPerformance, "new_launch_flight": final.Flight, "pilot_before": json.RawMessage(before), "committed_state": reopened.State(), "phoenix": backup.State.SelectedPilot(), "sortie_id": resultID, "strict_playback_done": q.Done, "strict_playback_cursor": q.Cursor, "music_cue": proposal.Result.MusicCue}
		b, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "new-generated-result.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("actual seed%d profile%s five Blackhawks: new complete WOC25; base return/result%d cue1b pages%v; %d new recorded ops strict; finalack/restart", in.Seed, profile, expectedScore, pages, len(fresh.Operations))
}

func careerContains(xs []string, want string) bool {
	for _, s := range xs {
		if s == want {
			return true
		}
	}
	return false
}

// These are ordinary generated missions with explicit source-complete WOC
// histories. The nonpickup case labels a supplied base approach; voluntary end
// is the normal user action. Neither case injects objective completion.
func TestNewWOCNonpickupAndUnsuccessfulEndings(t *testing.T) {
	for _, voluntary := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonpickup-base", true: "unsuccessful-voluntary"}[voluntary], func(t *testing.T) {
			c := missionController(t)
			a, e := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
			if e != nil {
				t.Fatal(e)
			}
			c.Office.Assets, c.Menus = a, a.Menus
			next := c.Store.State()
			pilot := next.SelectedPilot()
			pilot.Rank, pilot.MissionType, pilot.Theatre = gamestate.RankCaptain, gamestate.MissionFlight, gamestate.TheatrePersianGulf
			pilot.SquadronPilots = make([]gamestate.SquadronPilot, 4)
			for i, name := range []string{"Adams", "Allen", "Andrews", "Anderson"} {
				pilot.SquadronPilots[i] = gamestate.SquadronPilot{Name: name, Rank: gamestate.RankWOC, Portrait: i + 1, LegacyRaw: make([]byte, 41)}
			}
			if e = c.Store.Commit(next); e != nil {
				t.Fatal(e)
			}
			seed := int64(1)
			c.MissionSeed = &seed
			readyMission(t, c)
			startRendering(t, c)
			jFinishFade(t, c)
			if c.Mission.Data.Objectives["primary"].DisplayKind == 2 {
				t.Fatal("nonpickup fixture generated pickup")
			}
			before := filmJSON(c.Store.State().SelectedPilot())
			if voluntary {
				if e = c.Handle(Actions{EndMission: true}); e != nil {
					t.Fatal(e)
				}
				if e = c.Handle(Actions{ConfirmQuit: true}); e != nil {
					t.Fatal(e)
				}
			} else {
				c.DevelopmentScenario = &DevelopmentScenario{Name: "p2-labelled-nonpickup-base-approach"}
				if e = c.developmentScenarioPose(c.Rendering.ResultTargets.Records[0], true, 2, 1, 0); e != nil {
					t.Fatal(e)
				}
				scenarioSafeReturn(t, c)
			}
			if c.Screen != ScreenDebrief || c.Debrief.Pending.Proposal == nil {
				t.Fatal("WOC ordinary ending failed", c.Screen, c.Message)
			}
			proposal := c.Debrief.Pending.Proposal
			if proposal.Score.NormalizedFlags&1 != 0 {
				t.Fatal("invented successful primary")
			}
			if !bytes.Equal(before, filmJSON(c.Store.State().SelectedPilot())) {
				t.Fatal("ending changed career before finalack")
			}
			loopAcknowledge(t, c)
			reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
			if e != nil || !reflect.DeepEqual(c.Store.State(), reopened.State()) {
				t.Fatal("ending restart", e)
			}
		})
	}
}
