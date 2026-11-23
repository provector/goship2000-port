package menus

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func loopAcknowledge(t *testing.T, c *FrontEnd) []string {
	t.Helper()
	pages := []string{}
	for i := 0; i < 25 && c.Screen == ScreenDebrief; i++ {
		pages = append(pages, c.Debrief.Page)
		c.Frame(false)
		if c.Debrief.Page == "crew" {
			c.Debrief.CrewSelection = len(c.Debrief.Pending.Proposal.Result.Recommendations)
		}
		c.Debrief.Tick = 400 // Labelled semantic animation skip, no result mutation.
		if err := c.Handle(Actions{Enter: true}); err != nil {
			t.Fatal(err)
		}
		if err := c.Handle(Actions{}); err != nil {
			t.Fatal(err)
		}
	}
	// Step O inserts the Film Library after durable acknowledgement. EXIT, not
	// drawing or playback, consumes its session-only Campaign/office successor.
	if c.Screen != ScreenFilms {
		t.Fatalf("committed debrief did not reach Mission Films: %s %v", c.Screen, pages)
	}
	if err := c.activateFilmControl(18); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOffice && c.Screen != ScreenInterstitial && c.Screen != ScreenBriefing {
		t.Fatalf("debrief did not reach its committed office/Campaign successor: %s %v", c.Screen, pages)
	}
	if c.Screen != ScreenOffice && (c.Mission == nil || c.Mission.Data.Inputs.MissionType != gamestate.MissionCampaign || c.Debrief != nil) {
		t.Fatal("only an admitted continuing Campaign may bypass office")
	}
	return pages
}

func TestGenuinePreparedLoopsAndRestart(t *testing.T) {
	for _, name := range []string{"return", "damaged-return", "crash", "flight-return", "crew-choice", "outcome-rescue", "outcome-mia", "campaign-return", "campaign-win"} {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			p, r := c.Rendering, c.Rendering.Flight
			before := c.Store.State()
			if c.Mission.ResultLaunch == nil {
				t.Fatal(c.Mission.ResultUnavailable)
			}
			if name == "outcome-rescue" || name == "outcome-mia" {
				p.Initialized.EndSortieForQoL()
				if err := c.finishSortieResult(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := c.Handle(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				if err := c.AdvanceRendering(.12); err != nil {
					t.Fatal(err)
				}
				if name == "crew-choice" && p.Paused {
					if err := c.Handle(Actions{Cancel: true}); err != nil {
						t.Fatal(err)
					}
				}
				if name != "crash" {
					if err := c.Handle(Actions{}); err != nil {
						t.Fatal(err)
					}
					if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 60 && c.Screen == ScreenScene; i++ {
					if err := c.AdvanceRendering(.12); err != nil {
						t.Fatal(err)
					}
				}
				if c.Screen == ScreenQuit {
					if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if c.Screen != ScreenDebrief {
				t.Fatalf("termination not admitted: %s %s", c.Screen, c.Message)
			}
			pending, err := c.Store.PendingSortie()
			if err != nil {
				t.Fatal(err)
			}
			proposal := pending.Proposal
			if proposal.Result == nil || proposal.Facts.ResultContext == nil {
				t.Fatal("source result context absent")
			}
			afterStage := c.Store.State()
			before.Revision = afterStage.Revision
			if !reflect.DeepEqual(before, afterStage) {
				t.Fatal("pre-ack career mutation")
			}
			if name == "crash" && proposal.AcceptedPilot.Status != gamestate.StatusKIA {
				t.Fatal("source10 not KIA", proposal.Result.NormalizedFlags)
			}
			if name == "outcome-mia" && proposal.AcceptedPilot.Status != gamestate.StatusMIA {
				t.Fatal("seed99 far-normalizer did not produce MIA", proposal.Result.NormalizedFlags, proposal.Result.Threshold)
			}
			if name == "outcome-rescue" && proposal.Result.NormalizedFlags[0]&0x20 == 0 {
				t.Fatal("far source20 rescue absent", proposal.Result.NormalizedFlags)
			}
			if name == "flight-return" && len(proposal.Result.Crew) != 4 {
				t.Fatal("real crew rows absent")
			}
			raw, _ := json.Marshal(proposal)
			for i := 0; i < 3; i++ {
				c.Frame(false)
				_ = c.Tick()
			}
			pending, _ = c.Store.PendingSortie()
			again, _ := json.Marshal(pending.Proposal)
			if string(raw) != string(again) {
				t.Fatal("draw rerolled result")
			}
			pages := loopAcknowledge(t, c)
			committed := c.Store.State()
			if rec, err := c.Store.RecoveryCandidate(); err != nil || rec == nil {
				t.Fatal("Phoenix missing", err)
			}
			if err := c.Store.CommitSortie(proposal.Facts.SortieID); err != nil {
				t.Fatal(err)
			}
			reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(committed, reopened.State()) {
				t.Fatal("commit/restart changed canonical state")
			}
			if out := os.Getenv("GS2000_N4_EVIDENCE"); out != "" {
				if err := os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				b, _ := json.MarshalIndent(map[string]any{"scenario": c.DevelopmentScenario, "proposal": proposal, "pages": pages, "final": committed, "damage_events": r.DamageEvents}, "", "  ")
				if err := os.WriteFile(filepath.Join(out, name+".json"), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRetainedCampaignCrossmodePreservesFlightWithoutResult(t *testing.T) {
	c := developmentScenarioController(t, "return")
	draft := developmentScenarioController(t, "campaign-return").Mission.CampaignDraft
	if err := c.Store.AbandonSortie(c.Mission.ResultLaunch.Start.ID, "labelled external crossmode fixture"); err != nil {
		t.Fatal(err)
	}
	next := c.Store.State()
	next.SelectedPilot().Campaign = draft
	if err := c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	p := c.Rendering
	if err := c.beginSortieResult(c.Mission.Final); err != nil {
		t.Fatal(err)
	}
	if c.Mission.ResultLaunch != nil || c.Mission.ResultUnavailable == "" || c.Rendering != p || c.Screen != ScreenScene {
		t.Fatal("unsupported retained cache owner widened or lost flight")
	}
}
