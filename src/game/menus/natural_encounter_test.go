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

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// This is the ordinary saved-document BEGIN/preflight/simulation controller.
// The fixture was authored with placement/compiler/store APIs. No actor pose,
// contact, projectile, hit or result is supplied by this test.
func naturalController(t *testing.T, disabled bool) *FrontEnd {
	t.Helper()
	return naturalControllerProfile(t, disabled, replay.R21AuthoredCompatibility)
}

func naturalControllerProfile(t *testing.T, disabled bool, profile string) *FrontEnd {
	t.Helper()
	assets, fixture := os.Getenv("GS2000_TEST_MISSIONS"), os.Getenv("GS2000_R21_WITNESS")
	if assets == "" || fixture == "" {
		t.Skip("R21 prepared assets and fresh authored witness required")
	}
	a, err := assetdata.LoadFrontEndAssets(assets)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	err = filepath.WalkDir(filepath.Join(fixture, "saves"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(filepath.Join(fixture, "saves"), path)
		if e != nil {
			return e
		}
		target := filepath.Join(root, "saves", rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("witness symlink")
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if len(b) > 1<<20 {
			return fmt.Errorf("unbounded witness")
		}
		return os.WriteFile(target, b, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(filepath.Join(root, "saves"), filepath.Join(root, "no-original"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	c.Runtime.DisableEnemyAI = disabled
	c.Runtime.DisableWingmenAI = true // Explicit independent policy, not hostile eligibility.
	c.FilmDirectory = filepath.Join(root, "films")
	c.MissionDirectory = filepath.Join(root, "missions")
	files, err := filepath.Glob(filepath.Join(fixture, "missions", "*.g2m"))
	if err != nil || len(files) != 1 {
		t.Fatal("saved witness", err, files)
	}
	id := strings.TrimSuffix(filepath.Base(files[0]), ".g2m")
	doc, _, err := (gamestate.AuthoredStore{Directory: filepath.Join(fixture, "missions")}).Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.beginAuthoredMission(doc); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
	// Preserve the R21 witness's historical launch interpretation as new live
	// launches advance to R22. The empty profile remains the normal new launch.
	if profile != "" {
		c.simulationProfile = profile
		switches, e := gamestate.LaunchSwitches(c.Mission.Data, c.Mission.Settings, a.Missions.Generation.Tables.Initialization.CPGMapping)
		if e != nil {
			t.Fatal(e)
		}
		c.enemyLaunchRules, e = freezePreparedEnemyRules(a.Missions, c.Mission.Data, switches, disabled)
		if e != nil {
			t.Fatal(e)
		}
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Rendering.Initialized.EnemyRules == nil || c.Rendering.Initialized.EnemyRules.Disabled != disabled {
		t.Fatal("effective launch rules")
	}
	return c
}

func TestNaturalSavedEncounterResultReplayAndPolicy(t *testing.T) {
	output := os.Getenv("GS2000_R21_OUTPUT")
	if output == "" {
		output = os.Getenv("GS2000_R21_OUT")
	}
	type sample struct {
		Disabled                  bool
		Shots, AdvanceCalls, Hits int
		SimulationSteps           uint64
		ElapsedDA                 uint16
		Damaged                   bool
		Film                      string
		Score                     int16
		Actor                     int
	}
	var records []sample
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			c := naturalController(t, disabled)
			p := c.Rendering
			r := p.Flight
			before, _ := json.Marshal(c.Store.State().SelectedPilot())
			if c.Mission.ResultLaunch == nil {
				t.Fatal("normal sortie result capability missing")
			}
			sortieID := c.Mission.ResultLaunch.Start.ID
			actor := -1
			for i, e := range p.Initialized.Entities {
				if e.Kind == 6 && e.Object.Raw[11] == 0x55 && e.DynamicRaw[2] == 0 {
					actor = i
					break
				}
			}
			if actor < 0 {
				t.Fatal("actual authored Fixed Gun missing")
			}
			c.Handle(Actions{})
			c.Handle(Actions{FlightCommands: []int{2}})
			shots, steps := 0, 0
			misses := map[string]int{}
			damaged := false
			for steps = 0; steps < 1800 && c.Rendering != nil; steps++ {
				if steps < 32 {
					c.Handle(Actions{FlightCommands: []int{0}})
				}
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
				if p.Error != nil || r.Failure != "" {
					t.Fatal("natural encounter stopped", p.Error, r.Failure)
				}
				for _, e := range r.CombatEvents {
					if e.Kind == "hostile-miss" {
						misses[e.Source]++
					}
					if e.Kind == "launch" && e.Actor == actor && e.Slot >= 20 {
						shots++
					}
				}
				r.CombatEvents = nil // bounded diagnostic retention only; excluded from checkpoints.
				for _, h := range r.DamageEvents {
					if h.DamageBefore != h.DamageAfter {
						damaged = true
					}
				}
				if !disabled && shots > 0 && damaged {
					break
				}
				if disabled && steps >= 600 {
					break
				}
			}
			if disabled {
				if shots != 0 || len(r.DamageEvents) != 0 {
					t.Fatal("disabled hostile decisions", shots, r.DamageEvents)
				}
			} else if shots == 0 || !damaged {
				t.Fatalf("natural attack missing: shots%d damage%v steps%d misses%v player%+v contact%+v", shots, damaged, steps, misses, *r.State, r.Avionics.Contacts[actor])
			}
			// Change current preferences after the decision. Running and recorded
			// rules must remain the launch's values.
			c.Runtime.DisableEnemyAI = !disabled
			if p.Initialized.EnemyRules.Disabled != disabled {
				t.Fatal("running policy mutated")
			}
			if c.Rendering != nil {
				editorAction(t, c, Actions{EndMission: true})
				editorAction(t, c, Actions{ConfirmQuit: true})
			}
			if c.Screen != ScreenDebrief {
				t.Fatal("normal ending", c.Screen, c.Message)
			}
			staged, _ := json.Marshal(c.Store.State().SelectedPilot())
			if !bytes.Equal(before, staged) {
				t.Fatal("career changed before acknowledgment")
			}
			for i := 0; i < 64 && c.Screen != ScreenFilms; i++ {
				editorAction(t, c, Actions{Enter: true})
				editorAction(t, c, Actions{MenuEnd: true})
				editorAction(t, c, Actions{Enter: true})
			}
			if c.Screen != ScreenFilms || len(c.Library.Entries) != 1 {
				t.Fatal("finalack/film", c.Screen, c.Message, c.filmError)
			}
			f, err := c.Library.Store.Load(c.Library.Entries[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if f.Header.Simulation != replay.R21AuthoredCompatibility {
				t.Fatal(f.Header.Simulation)
			}
			if !disabled {
				var altered filmLaunch
				if err = json.Unmarshal(f.Launch, &altered); err != nil {
					t.Fatal(err)
				}
				altered.EnemyRules.Raw[0][0] ^= 1
				altered.EnemyRules, err = gamestate.FreezeEnemyRules(altered.EnemyRules.World, altered.EnemyRules.Raw, altered.EnemyRules.Switches, false)
				if err != nil {
					t.Fatal(err)
				}
				bad := *f
				bad.Launch = filmJSON(altered)
				if _, err = NewFilmPlayback(c.Office.Assets, &bad, c.Runtime); err == nil {
					t.Fatal("self-consistent unsupported world bank admitted")
				}
			}
			after, _ := json.Marshal(c.Store.State())
			afterPilot, _ := json.Marshal(c.Store.State().SelectedPilot())
			if bytes.Equal(before, afterPilot) {
				t.Fatal("no ordinary career result committed")
			}
			reopened, err := gamestate.OpenSaveStore(c.Store.Directory, filepath.Join(t.TempDir(), "no-original"))
			if err != nil {
				t.Fatal(err)
			}
			restarted, _ := json.Marshal(reopened.State())
			if !bytes.Equal(after, restarted) {
				t.Fatal("restart changed committed result")
			}
			if err = reopened.CommitSortie(sortieID); err != nil {
				t.Fatal("repeated acknowledgment", err)
			}
			duplicate, _ := json.Marshal(reopened.State())
			if !bytes.Equal(after, duplicate) {
				t.Fatal("duplicate acknowledgment changed career")
			}
			opt := c.Runtime
			opt.DisableEnemyAI = !disabled
			q, err := NewFilmPlayback(c.Office.Assets, f, opt)
			if err != nil {
				t.Fatal(err)
			}
			for !q.Done {
				if err = q.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if q.Preview().Initialized.EnemyRules.Disabled != disabled {
				t.Fatal("playback consulted current SETUP")
			}
			practice, err := NewFilmPlayback(c.Office.Assets, f, opt)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3 && !practice.Done; i++ {
				if err = practice.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if !disabled {
				for !practice.Done && len(practice.Preview().Flight.DamageEvents) == 0 {
					if err = practice.Step(); err != nil {
						t.Fatal(err)
					}
				}
				if practice.Done {
					t.Fatal("missing reached hit checkpoint before Resume")
				}
			}
			if err = practice.Resume(); err != nil {
				t.Fatal(err)
			}
			if practice.Engine.Store != nil || practice.Preview().Initialized.EnemyRules.Disabled != disabled {
				t.Fatal("Resume authority/rules")
			}
			if err = practice.AdvancePractice(.1); err != nil {
				t.Fatal(err)
			}
			unchanged, _ := json.Marshal(c.Store.State())
			if !reflect.DeepEqual(after, unchanged) {
				t.Fatal("isolated playback changed career")
			}
			record := sample{Disabled: disabled, Shots: shots, AdvanceCalls: steps, Hits: len(r.DamageEvents), SimulationSteps: r.Steps, ElapsedDA: r.ElapsedDA, Damaged: damaged, Film: f.Header.ID, Score: f.End.Score, Actor: actor}
			records = append(records, record)
			t.Logf("natural witness %+v", record)
			if out := output; out != "" {
				if err = os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				b, err := replay.Marshal(f)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(out, fmt.Sprintf("natural-disabled-%v.g2r", disabled)), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if out := output; out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.MarshalIndent(records, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "natural-result.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
