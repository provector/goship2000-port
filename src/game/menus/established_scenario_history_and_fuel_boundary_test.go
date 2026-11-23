package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

func TestEstablishedScenarioHistoryAndFuelBoundary(t *testing.T) {
	for _, name := range []string{"established-failure", "crew-choice-250", "fuel-empty", "inactive-phoenix", "mia-objective-recovery"} {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			p, r, meta := c.Store.State().SelectedPilot(), c.Rendering.Flight, c.DevelopmentScenario
			if p.Rank != gamestate.RankCaptain || p.Statistics.CareerScore != 7900 || p.Statistics.MissionCampaignCounter != 12 || p.LegacyRaw[0x54] != 7 || p.LegacyRaw[0x55] != 5 || p.LegacyRaw[0x56] != 1 || p.Statistics.AwardCounts[7] != 1 || len(p.SquadronPilots) != 4 || meta.Profile == "" || meta.Expected == "" {
				t.Fatal("established history missing or typed/retained owners disagree", p, meta)
			}
			if binary.LittleEndian.Uint32(p.LegacyRaw[0x72:]) != 7900 || binary.LittleEndian.Uint16(p.LegacyRaw[0x59:]) != 300 {
				t.Fatal("source score/penalty snapshots not synchronized")
			}
			if name == "crew-choice-250" {
				if len(p.SquadronPilots) != 4 || p.SquadronPilots[0].Rank != gamestate.RankCW2 || p.SquadronPilots[1].Rank != gamestate.RankWO1 || p.SquadronPilots[0].LegacyRaw[31] != 1 || c.Mission.ResultLaunch.Coefficient != 10 || c.Mission.Settings.EnemyQuality != 2 {
					t.Fatal("source-backed established crew/quality/coefficient", p.SquadronPilots, c.Mission.ResultLaunch.Coefficient)
				}
			}
			if name == "fuel-empty" {
				if c.Mission.Settings.FlightModel != 1 || c.Rendering.Initialized.Switches&4 == 0 || r.Systems.Fuel[0] != 1 || r.Systems.Countdown != 0 || len(r.acceptedPlayerHits) != 0 || r.State.Terminal != nil || r.Systems.Damage[9] != 0 {
					t.Fatal("ordinary fuel scenario became accepted damage or preset terminal")
				}
				if err := c.Handle(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				if err := c.AdvanceRendering(.25); err != nil {
					t.Fatal(err)
				}
				if r.Systems.Fuel[0] != 0 || r.Systems.Damage[9] != 0 || r.State.Terminal != nil || c.Screen != ScreenScene {
					t.Fatal("first ordinary1->0 pass short-circuited power/terminal outcome")
				}
			}
		})
	}
}

func TestReachedFollowerOldemptyExcludesUnprovenResult(t *testing.T) {
	c := developmentScenarioController(t, "flight-return")
	p, r := c.Rendering, c.Rendering.Flight
	before := c.Store.State()
	r.Systems.Fuel[1], r.Systems.Countdown = 1, 0
	r.Systems.Advance(true, 16)
	if r.Systems.UnsupportedFollowerEmptyBurn != 0 {
		t.Fatal("positive1->0 is not source follower oldempty")
	}
	r.Systems.Countdown = 0
	r.Systems.Advance(true, 16) // Real shared entry, supplied actor fuel/control only.
	p.Initialized.EndSortieForQoL()
	if err := c.finishSortieResult(); err != nil {
		t.Fatal(err)
	}
	waitForResultVerification(t, c)
	if c.Debrief != nil || c.Screen != ScreenFilms || c.Library == nil || !c.Library.Return.RetainedBriefing || c.Library.Notice == "" || !strings.Contains(c.Message, "aircraft 1 outcome unresolved: fuel exhaustion") || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) {
		t.Fatal("missing follower RNG/descent was admitted as a fabricated healthy or loss result", c.Message)
	}
	// R1.1 separates a rejected career from its recording evidence and returns
	// to the exact offered preparation after the native status/Library screen.
	mission := c.Mission
	if err := c.Handle(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.exitFilmLibrary(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing || c.Mission != mission || !reflect.DeepEqual(before.SelectedPilot(), c.Store.State().SelectedPilot()) {
		t.Fatal("rejected result lost preparation or changed career")
	}
}

func TestRealPickupFinalFactsIndependentOfPresentationSchedule(t *testing.T) {
	var expected []byte
	for _, mode := range []string{"smooth", "stepped"} {
		for _, scale := range []int{1, 2} {
			for _, fps := range []int{30, 60, 144} {
				t.Run(fmt.Sprintf("%s/%d/%d", mode, scale, fps), func(t *testing.T) {
					c := developmentScenarioController(t, "crew-choice-250")
					p, r := c.Rendering, c.Rendering.Flight
					seed := uint32(12345) // Labelled supplied result seed; not GS2 gameplay RNG.
					c.Mission.ResultLaunch.DevelopmentSeed = &seed
					p.Scene.RenderScale, p.presentation.mode = scale, mode
					p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
					p.resetPresentation()
					logical := func() []byte {
						var audio [][2]uint16
						for _, a := range r.sceneAudio {
							audio = append(audio, [2]uint16{a.event, a.argument})
						}
						b, err := json.Marshal(struct {
							Flight  flight.State
							Systems flight.Systems
							Targets *gamestate.MissionTargets
							Owner   [2]uint16
							RNG     uint16
							Steps   uint64
							Elapsed uint16
							Audio   [][2]uint16
						}{*r.State, r.Systems, p.ResultTargets, p.Initialized.ObjectiveAssignments, p.Initialized.RandomState, r.Steps, r.ElapsedDA, audio})
						if err != nil {
							t.Fatal(err)
						}
						return b
					}
					stepFrame := func() {
						if err := c.AdvanceRendering(1 / float64(fps)); err != nil {
							t.Fatal(err)
						}
						before := logical()
						p.Dirty = true
						c.Frame(false)
						c.Frame(false) // An extra draw may not add a logical consumer.
						if !bytes.Equal(before, logical()) {
							t.Fatal("drawing mutated authoritative flight/objective/RNG/audio state")
						}
					}
					if err := c.Handle(Actions{Cancel: true}); err != nil {
						t.Fatal(err)
					}
					for i := 0; i < fps*2 && c.DevelopmentScenario.Stage != "loaded-return"; i++ {
						stepFrame()
					}
					if c.DevelopmentScenario.Stage != "loaded-return" || r.Steps != 1 {
						t.Fatal("actual pickup/staged pause logical boundary changed")
					}
					if err := c.Handle(Actions{Cancel: true}); err != nil {
						t.Fatal(err)
					}
					if err := c.Handle(Actions{}); err != nil {
						t.Fatal(err)
					}
					if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
						t.Fatal(err)
					}
					for i := 0; i < fps*5 && c.Screen == ScreenScene; i++ {
						stepFrame()
					}
					if c.Screen != ScreenQuit {
						t.Fatal("actual return did not reach source prompt")
					}
					if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
						t.Fatal(err)
					}
					proposal := c.Debrief.Pending.Proposal
					if proposal.Score.Score != 250 || proposal.Facts.SourceFlags&0x81 != 0x81 {
						t.Fatal("presentation test failed real source250 completion")
					}
					facts := proposal.Facts
					facts.SortieID = "comparison-only" // The only random identity in these frozen facts.
					b, err := json.Marshal(struct {
						Logical json.RawMessage
						Facts   gamestate.SortieFacts
						Score   gamestate.SortieScore
						Result  *gamestate.SortieResultDetails
					}{logical(), facts, proposal.Score, proposal.Result})
					if err != nil {
						t.Fatal(err)
					}
					if expected == nil {
						expected = b
					} else if !bytes.Equal(expected, b) {
						t.Fatal("real final facts/score/RNG decisions changed with presentation schedule")
					}
				})
			}
		}
	}
}

func scenarioSafeReturn(t *testing.T, c *FrontEnd) {
	t.Helper()
	if c.Rendering.Paused {
		if err := c.Handle(Actions{Cancel: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60 && c.Screen == ScreenScene; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenQuit {
		t.Fatalf("natural contact/rotor prompt missing: %s %s", c.Screen, c.Message)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenDebrief {
		t.Fatalf("source return not admitted: %s %s", c.Screen, c.Message)
	}
}

func TestCrew250UsesActualPickupAndReturn(t *testing.T) {
	c := developmentScenarioController(t, "crew-choice-250")
	before := c.Store.State().SelectedPilot()
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.25); err != nil {
		t.Fatal(err)
	}
	if c.DevelopmentScenario.Stage != "loaded-return" || c.Rendering.Initialized.ObjectiveAssignments[0] != 1 {
		t.Fatal("actual player pickup did not own staged return")
	}
	scenarioSafeReturn(t, c)
	p := c.Debrief.Pending.Proposal
	if p.Score.Score != 250 || p.Score.AircraftContribution != 150 || p.Score.CareerApplied == false || p.Score.NormalizedFlags&0x81 != 0x81 || p.Facts.SelectorMultisetMatch || p.InitialRankRecommendation != 0 || len(p.Result.Recommendations) == 0 {
		t.Fatal("source250 facts/progression", p.Score, p.InitialRankRecommendation, p.Result.Recommendations)
	}
	if !reflect.DeepEqual(before, c.Store.State().SelectedPilot()) {
		t.Fatal("unacknowledged result mutated persisted pilot")
	}
}

func TestEstablishedFailureAndCampaignDefeatActualReturn(t *testing.T) {
	for _, name := range []string{"established-failure", "campaign-defeat"} {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			scenarioSafeReturn(t, c)
			p := c.Debrief.Pending.Proposal
			if p.Success || p.Score.Score != 0 || p.Score.NormalizedFlags&0x83 != 0x80 || p.Result.MusicCue != 0x1c && name != "campaign-defeat" || binary.LittleEndian.Uint16(p.ScoredPilot.LegacyRaw[0x59:]) != 500 {
				t.Fatal("source failed-return score/flags/penalty/music", p.Score, p.Result.MusicCue)
			}
			if name == "campaign-defeat" && (p.Result.Campaign == nil || p.Result.Campaign.Verdict != -1 || !p.Result.Campaign.Concluded || p.Result.MusicCue != 0x1f) {
				t.Fatal("real unsuccessful return did not produce matched Campaign defeat")
			}
		})
	}
}

func TestMIARecoveryScenarioUsesRealPickupAndReturn(t *testing.T) {
	c := developmentScenarioController(t, "mia-objective-recovery")
	target := c.Store.State().Slots[1]
	if target == nil || target.Status != gamestate.StatusMIA || target.LegacyRaw[0x5b] != byte(c.Mission.Data.SourceWorld.Suffix) {
		t.Fatal("unique supplied prior MIA scenario identity")
	}
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceRendering(.25); err != nil {
		t.Fatal(err)
	}
	scenarioSafeReturn(t, c)
	if c.Debrief.Pending.Proposal.RecoveredPilot == nil || c.Debrief.Pending.Proposal.RecoveredPilot.Before.ID != target.ID {
		t.Fatal("real completed pickup/safe return did not propose unique MIA compatibility recovery")
	}
	if c.Store.State().Slots[1].Status != gamestate.StatusMIA {
		t.Fatal("prior MIA changed before final acknowledgement")
	}
	loopAcknowledge(t, c)
	recovered := c.Store.State().Slots[1]
	if recovered.ID != target.ID || recovered.Status != gamestate.StatusActive || recovered.Rank != target.Rank || recovered.Statistics.CareerScore != target.Statistics.CareerScore {
		t.Fatal("compatibility recovery did not atomically change only the supported other-pilot status")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !reflect.DeepEqual(c.Store.State(), reopened.State()) {
		t.Fatal("two-pilot real-pickup recovery changed on restart", err)
	}
}

func TestPrimaryScenarioRealFiringReachesStagedReturn(t *testing.T) {
	c := developmentScenarioController(t, "established-primary")
	fireScenarioObjectives(t, c)
	scenarioSafeReturn(t, c)
	if !c.Debrief.Pending.Proposal.Success || c.Debrief.Pending.Proposal.Facts.SourceFlags&0x81 != 0x81 || c.Debrief.Pending.Proposal.Score.Score <= 0 {
		t.Fatal("real objective did not reach successful source proposal")
	}
}

func TestCampaignVictoryUsesRealBothObjectiveMechanics(t *testing.T) {
	c := developmentScenarioController(t, "campaign-victory")
	fireScenarioObjectives(t, c)
	scenarioSafeReturn(t, c)
	proposal := c.Debrief.Pending.Proposal
	if !proposal.Success || proposal.Score.NormalizedFlags&0x83 != 0x83 || proposal.Result.Campaign == nil || proposal.Result.Campaign.Quality != 200 || proposal.Result.Campaign.Verdict != 1 || !proposal.Result.Campaign.Concluded || proposal.Result.MusicCue != 0x1e {
		t.Fatal("real both-objective Campaign win facts/art/music", proposal.Score, proposal.Result.Campaign, proposal.Result.MusicCue)
	}
	if c.Debrief.Pending.Proposal.PilotBefore.Campaign != nil {
		t.Fatal("controlled threshold was substituted from a stale PilotBefore Campaign")
	}
	id, score := proposal.Facts.SortieID, proposal.Score.Score
	loopAcknowledge(t, c)
	committed := c.Store.State()
	if c.Screen != ScreenOffice || committed.SelectedPilot().Statistics.CareerScore != 7900+score || committed.SelectedPilot().MissionType != gamestate.MissionFlight {
		t.Fatal("real Campaign success was not committed through its actual debrief controller")
	}
	if err := c.Store.CommitSortie(id); err != nil || !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("duplicate victory acknowledgement changed durable progression", err)
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || !reflect.DeepEqual(reopened.State(), committed) {
		t.Fatal("real objective-to-commit Campaign result changed on restart", err)
	}
}

func fireScenarioObjectives(t *testing.T, c *FrontEnd) {
	t.Helper()
	p, r := c.Rendering, c.Rendering.Flight
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	tick := func(a Actions) {
		t.Helper()
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	tick(Actions{WeaponTriggerValid: true})
	for i := 0; i < 4; i++ {
		station := p.Initialized.Stores.Ships[0].Stations[p.Initialized.SelectedStation]
		if strings.Contains(strings.ToUpper(station.ShortName), "HELL") {
			break
		}
		tick(Actions{OrdnanceNext: true, WeaponTriggerValid: true})
	}
	t.Logf("initial target%d stations%+v rows%v", r.Avionics.Target, p.Initialized.Stores.Ships[0].Stations, c.DevelopmentScenario.TargetRows)
	for i := 0; i < 1000 && c.DevelopmentScenario.Stage != "combat-return"; i++ {
		if p.Paused {
			if c.DevelopmentScenario.Stage != "secondary-combat" || !r.Combat.Primary.Achieved {
				t.Fatal("scenario paused without source completion")
			}
			tick(Actions{Cancel: true, WeaponTriggerValid: true})
		}
		if r.Avionics.Target == 0 || i%20 == 0 {
			tick(Actions{WeaponTriggerValid: true, Backspace: true})
		} else {
			tick(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
		}
	}
	if c.DevelopmentScenario.Stage != "combat-return" || !r.Combat.Primary.Achieved || p.ResultTargets.Records[2][32] != 0 {
		first := len(r.TriggerTrace) - 8
		if first < 0 {
			first = 0
		}
		t.Fatalf("actual firing failed to finish generated objective; stage%s target%d primary%+v secondary%+v last traces%+v", c.DevelopmentScenario.Stage, r.Avionics.Target, r.Combat.Primary, r.Combat.Secondary, r.TriggerTrace[first:])
	}
	launches, destroyed := 0, 0
	for _, e := range r.CombatEvents {
		if e.Kind == "launch" {
			launches++
		}
		if e.Kind == "destroyed" {
			destroyed++
		}
	}
	if launches == 0 || destroyed == 0 {
		t.Fatal("source completion lacked actual launch/destruction events")
	}
}
