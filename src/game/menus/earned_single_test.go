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
	"strings"
	"testing"
)

// Fresh generated Single with supplied qualified WO1/near-threshold history and
// a labelled initial hover approach. ORDNANCE, actual Hellfire collision/damage,
// objective credit, ending, commission decision and publication are production
// consumers. This is neither the missing human Single film nor continuous flight.
func TestEarnedSingleActualMechanicsAcceptRefuse(t *testing.T) {
	earnedSingleActualMechanics(t, "p3-first-commission", 1500, 128)
}

func TestEarnedSingleSafeApproachActualMechanics(t *testing.T) {
	earnedSingleActualMechanics(t, "p3-first-commission-safe-approach", 8000, 1024)
}

func earnedSingleActualMechanics(t *testing.T, scenario string, south uint32, above int16) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accept-rename", false: "refuse"}[accepted], func(t *testing.T) {
			var c *FrontEnd
			if south == 1500 {
				// Historical narrow benchmark predates victim-local wing stores. Its real
				// incoming hit now legitimately halves this player's Hellfires. Keep
				// the original input/commission witness under its recorded R5 rules;
				// the safe-approach neighbor continues to exercise current launches.
				c = developmentScenarioControllerProfile(t, scenario, replay.R5Compatibility)
			} else {
				c = developmentScenarioController(t, scenario)
			}
			a := c.Office.Assets
			p, r := c.Rendering, c.Rendering.Flight
			seed := int64(2)
			worldName := c.Mission.Data.World
			row := c.DevelopmentScenario.TargetRows[0]
			var err error
			var b []byte
			var initial filmLaunch
			if err = json.Unmarshal(p.filmRecorder.Film.Launch, &initial); err != nil {
				t.Fatal(err)
			}
			if initial.Adjustment == nil || initial.Adjustment.Approach == nil || initial.Adjustment.Approach.H0 != p.Initialized.Entities[row].Pose.H0 || initial.Adjustment.Approach.H1 != p.Initialized.Entities[row].Pose.H1-south || initial.Adjustment.Approach.AboveSupport != above || r.State.Pose.H1 != initial.Adjustment.Approach.H1 || r.State.Pose.Height != r.State.ContactHeight+above || c.DevelopmentScenario.Pose != r.State.Pose {
				t.Fatal("declared initial adjustment/metadata mismatch")
			}
			before := filmJSON(c.Store.State().SelectedPilot())
			tick := func(a Actions) {
				t.Helper()
				if err = c.Handle(a); err != nil {
					t.Fatal(err)
				}
				if err = c.AdvanceRendering(.12); err != nil {
					t.Fatal(err)
				}
				p.takeSceneAudio()
			}
			if p.Paused {
				tick(Actions{Cancel: true, WeaponTriggerValid: true})
			}
			for i := 0; i < 6; i++ {
				station := p.Initialized.Stores.Ships[0].Stations[p.Initialized.SelectedStation]
				if strings.Contains(strings.ToUpper(station.ShortName), "HELL") {
					break
				}
				tick(Actions{OrdnanceNext: true, WeaponTriggerValid: true})
			}
			for i := 0; i < 1500 && !r.Combat.Primary.Achieved && c.Screen == ScreenScene; i++ {
				if south == 1500 {
					// Retain the original narrow314-point benchmark input stream.
					if r.Avionics.Target == 0 || i%20 == 0 {
						tick(Actions{Backspace: true, WeaponTriggerValid: true})
					} else {
						tick(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
					}
					continue
				}
				selected := int(r.Avionics.Target) - 1 // AB9 is source one-based.
				valid := selected >= 0 && selected < len(p.Initialized.Entities) && p.Initialized.Entities[selected].Object.Raw[0]&0x10 != 0 && p.Initialized.Entities[selected].Kind&0x38 == 0
				if !valid {
					tick(Actions{Backspace: true, WeaponTriggerValid: true})
					continue
				}
				// A genuine longer-range shot keeps its selected primary while
				// flying. No new target is assigned and no ammo/slot is reset.
				inFlight := false
				for _, projectile := range r.Combat.Slots {
					inFlight = inFlight || projectile.Lifetime != 0 && projectile.Shooter == 0
				}
				if inFlight {
					tick(Actions{WeaponTriggerValid: true})
				} else {
					tick(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true, WeaponTriggerPressed: true})
				}
			}
			launches, destroyed := 0, 0
			for _, ev := range r.CombatEvents {
				if ev.Kind == "launch" {
					launches++
				}
				if ev.Kind == "destroyed" {
					destroyed++
				}
			}
			if !r.Combat.Primary.Achieved || launches == 0 || destroyed == 0 {
				t.Fatalf("actual generated objective not reached: world%s primary%+v target%d launches%d destroyed%d messages%s", c.Mission.Data.World, r.Combat.Primary, r.Avionics.Target, launches, destroyed, c.Message)
			}
			if err = c.Handle(Actions{EndMission: true}); err != nil {
				t.Fatal(err)
			}
			if err = c.Handle(Actions{ConfirmQuit: true}); err != nil {
				t.Fatal(err)
			}
			if c.Screen != ScreenDebrief || c.Debrief == nil {
				t.Fatal("ordinary produced result missing", c.Screen, c.Message)
			}
			proposal := c.Debrief.Pending.Proposal
			if !proposal.Success || !proposal.Commission || proposal.Facts.ActorCount != 1 || proposal.Score.Score < 100 {
				t.Fatal("earned source commission predicate not reached", proposal.Success, proposal.Commission, proposal.Score, proposal.Facts.SourceFlags)
			}
			if !bytes.Equal(before, filmJSON(c.Store.State().SelectedPilot())) {
				t.Fatal("pre-ack publication")
			}
			pages := []string{c.Debrief.Page}
			c.Debrief.Tick = 400
			if err = c.Handle(Actions{Enter: accepted, Cancel: !accepted}); err != nil {
				t.Fatal(err)
			}
			for c.Debrief.Page == "rescue" {
				pages = append(pages, "rescue")
				c.Debrief.Tick = 400
				if err = c.Handle(Actions{Enter: true}); err != nil {
					t.Fatal(err)
				}
			}
			if c.Debrief.Page != "jump" {
				t.Fatal("source commander decision route", c.Debrief.Page)
			}
			pages = append(pages, "jump")
			c.Debrief.Tick = 400
			if err = c.Handle(Actions{Enter: true}); err != nil {
				t.Fatal(err)
			}
			pages = append(pages, c.Debrief.Page)
			if accepted {
				if c.Debrief.Page != "replace" || len(c.Debrief.Pilot.SquadronPilots) != 4 {
					t.Fatal("earned accepted commission omitted actual REPLACE rows")
				}
				if err = c.Handle(Actions{Backspace: true}); err != nil {
					t.Fatal(err)
				}
				if err = c.Handle(Actions{Text: []rune("EARNED")}); err != nil {
					t.Fatal(err)
				}
				if err = c.Handle(Actions{Enter: true}); err != nil {
					t.Fatal(err)
				}
				if c.Debrief.Pilot.SquadronPilots[0].Name != "EARNED" {
					t.Fatal("real pending rename failed")
				}
			} else if c.Debrief.Page != "feedback" || len(c.Debrief.Pilot.SquadronPilots) != 0 {
				t.Fatal("refusal created replacement squadron")
			}
			id := proposal.Facts.SortieID
			acknowledgeLibrary(t, c)
			once := c.Store.State()
			if err = c.Store.CommitSortie(id); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(once, c.Store.State()) {
				t.Fatal("duplicate earned award")
			}
			reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
			if err != nil || !reflect.DeepEqual(once, reopened.State()) {
				t.Fatal("earned restart", err)
			}
			f, err := (replay.Store{Root: c.FilmDirectory}).Load(p.filmRecorder.Film.Header.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, scale := range []int{1, 2} {
				cfg := configuration.Defaults()
				cfg.SimulationRenderScale = scale
				q, err := NewFilmPlayback(a, f, cfg)
				if err != nil {
					t.Fatal(err)
				}
				for !q.Done {
					if err = q.Step(); err != nil {
						t.Fatal("new earned film strict", scale, err)
					}
				}
				if q.Engine.Store != nil {
					t.Fatal("viewer acquired career")
				}
			}
			if out := os.Getenv("GS2000_P3_OUTPUT"); out != "" {
				name := map[bool]string{true: "earned-single-accept", false: "earned-single-refuse"}[accepted]
				if south != 1500 {
					name = "safe-approach-" + name
				}
				if err = os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				b, _ = replay.Marshal(f)
				os.WriteFile(filepath.Join(out, name+".g2r"), b, 0600)
				b, _ = json.MarshalIndent(map[string]any{"scope": "new source-generated Single, dev-only supplied qualified WO1 career990 and initial approach; real Hellfire/goal/AltE/result/commission; not continuous flight or old human film", "scenario": scenario, "initial_approach": initial.Adjustment, "seed": seed, "target_row": row, "launches": launches, "destroyed": destroyed, "pages": pages, "proposal": proposal, "committed": once, "film_id": f.Header.ID, "end": f.End}, "", "  ")
				os.WriteFile(filepath.Join(out, name+".json"), b, 0600)
			}
			t.Logf("fresh Single%s seed%d score%d ops%d steps%d launches%d destroyed%d accepted%v pages%v", worldName, seed, proposal.Score.Score, len(f.Operations), f.End.Steps, launches, destroyed, accepted, pages)
		})
	}
}
