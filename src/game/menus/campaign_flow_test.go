package menus

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewCorridorCampaignRecordedResult(t *testing.T) {
	a, old, l, j := retainedCampaign(t)
	for _, success := range []bool{true, false} {
		name := "full-schedule-fuel-loss-failure"
		if !success {
			name = "early-voluntary-failure"
		}
		t.Run(name, func(t *testing.T) {
			c := missionController(t)
			// Reached controls from the retained noncombat witness exercise fuel
			// and result ownership under the accepted pre-R2 rules. New hostile
			// encounters have their own R21 fixture and NPC boundary tests.
			c.simulationProfile = replay.R12Compatibility
			c.Office.Assets, c.Menus, c.Runtime = a, a.Menus, l.Runtime
			saved := c.Store.State()
			pilot := j.Start.PilotBefore
			saved.Slots[0] = pilot
			saved.SelectedPilotID = pilot.ID
			if e := c.Store.Commit(saved); e != nil {
				t.Fatal(e)
			}
			s, e := prepareMissionSession(a.Missions, pilot, l.Final.Mission.Inputs.Seed, nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			final, e := cloneFilmFinal(&l.Final)
			if e != nil {
				t.Fatal(e)
			}
			final.Mission, final.Flight, final.Settings = s.Data, s.Flight, s.Settings
			s.Final = final
			s.ReadinessRows = final.Aircraft
			c.Mission = s
			c.Screen = ScreenScene
			if e = c.constructSimulation(final); e != nil {
				t.Fatal(e)
			}
			c.beginFilmRecording(final)
			p := c.Rendering
			r := p.Flight
			if p.filmRecorder == nil {
				t.Fatal("recording unavailable", c.Message)
			}
			before := filmJSON(c.Store.State().SelectedPilot())
			for i, op := range old.Operations {
				if !success && r.Steps >= 3 {
					if e = c.Handle(Actions{EndMission: true}); e != nil {
						t.Fatal(e)
					}
					if e = c.Handle(Actions{ConfirmQuit: true}); e != nil {
						t.Fatal(e)
					}
					break
				}
				switch op.Kind {
				case "step", "terminal":
					var v filmStep
					if e = json.Unmarshal(op.Payload, &v); e != nil {
						t.Fatal(e)
					}
					timing := r.Clock.ReplayTiming()
					if timing != v.Timing {
						t.Fatal("recomputedclock", i)
					}
					r.filmTiming = &timing
					r.Paused, p.Paused = false, false
					r.terminalAwaitingAck = false
					var commands []int
					if v.Input.HasCommand {
						commands = []int{v.Input.Command}
					}
					r.commandInput.samples = []flightInputSample{{at: 0, commands: commands, aux: v.Input.Aux, directions: v.Input.Directions, trigger: v.Input.Trigger, press: v.Input.Pressed}}
					if e = c.advanceFlight(0); e != nil {
						t.Fatal(i, e)
					}
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
						p.Initialized.AnswerNaturalEnd(true)
					}
					if e = c.finishSortieResult(); e != nil {
						t.Fatal(e)
					}
				}
				if p.filmRecorder.Error != "" {
					t.Fatal(i, p.filmRecorder.Error)
				}
				p.takeSceneAudio()
			}
			if c.Screen != ScreenDebrief || c.Debrief == nil {
				t.Fatalf("new corridor result rejected: %s %s fuel%v mask%x actors%v", c.Screen, c.Message, r.Systems.Fuel, r.Systems.UnsupportedFollowerEmptyBurn, p.Initialized.Entities[:5])
			}
			proposal := c.Debrief.Pending.Proposal
			if proposal.Success {
				b, _ := json.MarshalIndent(proposal, "", "  ")
				diagnostic := filepath.Join(t.TempDir(), "p3-corridor-outcome.json")
				if err := os.WriteFile(diagnostic, b, 0600); err != nil {
					t.Fatal(err)
				}
				t.Fatalf("source-produced outcome got%v want%v score%+v disqualifier%v goals%+v/%+v diagnostic=%s", proposal.Success, success, proposal.Score, campaignFlowDisqualifier(proposal.Facts), r.Combat.Primary, r.Combat.Secondary, diagnostic)
			}
			if r.Systems.UnsupportedFollowerEmptyBurn != 0 {
				t.Fatal("incomplete successor")
			}
			if !bytes.Equal(before, filmJSON(c.Store.State().SelectedPilot())) {
				t.Fatal("premature career publication")
			}
			fresh, e := c.SealRecordedFilm()
			if e != nil {
				t.Fatal(e)
			}
			if !fresh.FormationMechanics() || fresh.Header.ID == old.Header.ID {
				t.Fatal("newprofile/identity")
			}
			id := proposal.Facts.SortieID
			pages := acknowledgeLibrary(t, c)
			if c.Screen != ScreenFilms {
				t.Fatal("finalackroute", c.Screen)
			}
			once := c.Store.State()
			if e = c.Store.CommitSortie(id); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(once, c.Store.State()) {
				t.Fatal("duplicateaward")
			}
			reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
			if e != nil || !reflect.DeepEqual(once, reopened.State()) {
				t.Fatal("restart", e)
			}
			published, e := (replay.Store{Root: c.FilmDirectory}).Load(fresh.Header.ID)
			if e != nil {
				t.Fatal(e)
			}
			q, e := NewFilmPlayback(a, published, c.Runtime)
			if e != nil {
				t.Fatal(e)
			}
			for !q.Done {
				if e = q.Step(); e != nil {
					t.Fatal("new strict", e)
				}
			}
			if q.Engine.Store != nil {
				t.Fatal("playbackcareer capability")
			}
			if out := os.Getenv("GS2000_P3_OUTPUT"); out != "" {
				os.MkdirAll(out, 0700)
				b, _ := replay.Marshal(published)
				os.WriteFile(filepath.Join(out, name+".g2r"), b, 0600)
				b, _ = json.MarshalIndent(map[string]any{"scope": "fresh source-generated launch; recorded normalized real user inputs consumed anew, no old checkpoint/pose/goal assignment; not new OS flight", "proposal": proposal, "pages": pages, "committed": once, "end": published.End, "operations": q.Cursor, "events": q.EventCount, "fuel": r.Systems.Fuel, "actors": p.Initialized.Entities[:9]}, "", "  ")
				os.WriteFile(filepath.Join(out, name+".json"), b, 0600)
			}
			t.Logf("%s: score%d flags%x pages%v newfilm%s %dops strict", name, proposal.Score.Score, proposal.Score.NormalizedFlags, pages, published.Header.ID, q.Cursor)
		})
	}
}

func campaignFlowDisqualifier(f gamestate.SortieFacts) []int {
	var out []int
	for i, v := range f.ObjectValues {
		if (f.ObjectCredits[i/2]>>uint((i&1)*4))&15 == 1 && v < 0 {
			out = append(out, i)
		}
	}
	return out
}
func acknowledgeLibrary(t *testing.T, c *FrontEnd) []string {
	t.Helper()
	var pages []string
	for i := 0; i < 25 && c.Screen == ScreenDebrief; i++ {
		pages = append(pages, c.Debrief.Page)
		c.Frame(false)
		if c.Debrief.Page == "crew" {
			c.Debrief.CrewSelection = len(c.Debrief.Pending.Proposal.Result.Recommendations)
		}
		c.Debrief.Tick = 400
		if e := c.Handle(Actions{Enter: true}); e != nil {
			t.Fatal(e)
		}
		if e := c.Handle(Actions{}); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenFilms {
		t.Fatal("debriefblocked", c.Screen, pages)
	}
	return pages
}
