package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func enemyEncounterNaturalController(t *testing.T, fixture string, disabled, wingmenDisabled bool) *FrontEnd {
	return naturalControllerMode(t, fixture, disabled, wingmenDisabled, 0)
}
func naturalControllerMode(t *testing.T, fixture string, disabled, wingmenDisabled bool, generatedSeed int64, profiles ...string) *FrontEnd {
	t.Helper()
	assets := os.Getenv("GS2000_TEST_MISSIONS")
	if assets == "" || fixture == "" {
		t.Skip("R23 prepared assets and saved authored encounter required")
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
	if generatedSeed != 0 {
		supplied := store.State()
		supplied.SelectedPilot().MissionType = gamestate.MissionFlight
		if err = store.Commit(supplied); err != nil {
			t.Fatal(err)
		}
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	c.simulationProfile = replay.R23AuthoredCompatibility // Frozen R23 control, independent of newer default.
	if generatedSeed != 0 {
		c.simulationProfile = replay.R23Compatibility
	}
	if len(profiles) > 0 {
		c.simulationProfile = profiles[0]
	}
	c.Runtime.DisableEnemyAI = disabled
	c.Runtime.DisableWingmenAI = wingmenDisabled // Frozen independently; explicit orders still work.
	c.FilmDirectory = filepath.Join(root, "films")
	c.MissionDirectory = filepath.Join(root, "missions")
	if generatedSeed != 0 {
		if err = c.startMission(generatedSeed, nil); err != nil {
			t.Fatal(err)
		}
	} else {
		files, err := filepath.Glob(filepath.Join(fixture, "missions", "*.g2m"))
		if err != nil || len(files) != 1 {
			t.Fatal("saved witness", files, err)
		}
		id := strings.TrimSuffix(filepath.Base(files[0]), ".g2m")
		doc, _, err := (gamestate.AuthoredStore{Directory: filepath.Join(fixture, "missions")}).Load(id)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.beginAuthoredMission(doc); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}

	switches, e := gamestate.LaunchSwitches(c.Mission.Data, c.Mission.Settings, a.Missions.Generation.Tables.Initialization.CPGMapping)
	if e != nil {
		t.Fatal(e)
	}
	c.enemyLaunchRules, e = freezePreparedEnemyRules(a.Missions, c.Mission.Data, switches, disabled)
	if e != nil {
		t.Fatal(e)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if !c.Rendering.wingmanDamageMechanics || c.Rendering.Initialized.EnemyRules == nil || c.Rendering.Initialized.EnemyRules.Disabled != disabled {
		t.Fatal("frozen R23 rules missing")
	}
	return c
}

type encounterSample struct {
	Case              string           `json:"case"`
	Disabled          bool             `json:"enemy_ai_disabled"`
	WingmenDisabled   bool             `json:"wingmen_ai_disabled"`
	AdvanceCalls      int              `json:"advance_calls"`
	Shots             int              `json:"shots"`
	PlayerGuidedShots int              `json:"player_guided_shots"`
	Events            []encounterEvent `json:"events"`
}
type encounterEvent struct {
	Step  uint64 `json:"step"`
	Event any    `json:"event"`
}

func TestNormallyAuthoredEncounterFamilies(t *testing.T) {
	base := os.Getenv("GS2000_R23_WITNESSES")
	if base == "" {
		t.Skip("R23 saved editor fixtures required")
	}
	for _, name := range []string{"ground", "helicopter", "fixed", "guided"} {
		t.Run(name, func(t *testing.T) {
			enabledCalls := 0
			for _, disabled := range []bool{false, true} {
				t.Run(fmt.Sprint("disabled=", disabled), func(t *testing.T) {
					attempt := "try01"
					if name == "ground" || name == "guided" {
						attempt = "try02"
					}
					c := enemyEncounterNaturalController(t, filepath.Join(base, name, attempt, "app"), disabled, name == "guided")
					defer c.AbandonFilmRecording()
					p, r := c.Rendering, c.Rendering.Flight
					before, _ := json.Marshal(c.Store.State().SelectedPilot())
					sample := encounterSample{Case: name, Disabled: disabled, WingmenDisabled: name == "guided"}
					model := map[string]byte{"ground": 110, "helicopter": 48, "fixed": 86, "guided": 34}[name]
					actors := map[int]bool{}
					first := len(p.Initialized.Entities)
					for i, e := range p.Initialized.Entities {
						if e.Object.Raw[12] == model {
							actors[i] = true
							if i < first {
								first = i
							}
							t.Logf("initial actor%d kind%d P%d group%d attack%02x pose%+v", i, e.Kind, e.Participation, e.DynamicRaw[2], e.Object.Raw[11], e.Pose)
						}
					}
					if len(actors) == 0 {
						t.Fatal("expected model absent", model)
					}
					cold := p.Initialized.Entities[first]
					if name == "helicopter" || name == "fixed" {
						retained := false
						for _, instance := range p.instances {
							if instance.ID == cold.ID {
								retained = true
							}
						}
						if cold.Participation != 0 || cold.RawMotion != 0 || cold.Pose.Height != 0 || !retained {
							t.Fatal("cold actor/header registration", cold, retained)
						}
					}
					step := func() {
						t.Helper()
						if err := c.advanceFlight(.1); err != nil {
							t.Fatal(err)
						}
						sample.AdvanceCalls++
						if sample.AdvanceCalls == 1 && (name == "helicopter" || name == "fixed") {
							e := p.Initialized.Entities[first]
							if e.Participation != 1 || e.Pose.H0 != cold.Pose.H0 || e.Pose.H1 != cold.Pose.H1 || e.Pose.Height != 0x400 {
								t.Fatal("cold source first visit", e, cold)
							}
						}
						if p.Error != nil || r.Failure != "" {
							t.Fatal(p.Error, r.Failure)
						}
						for _, e := range r.CombatEvents {
							if actors[e.Actor] || e.Kind == "hostile-impact" || e.Kind == "hostile-miss" {
								sample.Events = append(sample.Events, encounterEvent{r.Steps, e})
								if actors[e.Actor] && e.Kind == "launch" {
									sample.Shots++
									if e.Target == 0 && e.Value == 9 {
										sample.PlayerGuidedShots++
									}
									t.Logf("natural %s step%d %+v", name, r.Steps, e)
								}
							}
						}
						r.CombatEvents = nil
					}
					c.Handle(Actions{})
					c.Handle(Actions{FlightCommands: []int{2}})
					for i := 0; i < 40; i++ {
						step()
					}
					if name == "guided" {
						editorAction(t, c, Actions{GPSOpen: true})
						for i := 0; i < 4; i++ {
							editorAction(t, c, Actions{GPSKey: "n"})
							editorAction(t, c, Actions{GPSKey: "h"})
						}
						editorAction(t, c, Actions{ViewKey: 1})
					}
					if name == "ground" || name == "guided" {
						for n := 0; n < 40; n++ {
							c.Handle(Actions{FlightCommands: []int{0}})
							step()
						}
						c.Handle(Actions{})
						c.Handle(Actions{FlightCommands: []int{10}})
						c.Handle(Actions{})
					} else {
						issueGunRoute(t, c, first)
					}
					maxCalls := 2400
					if disabled {
						maxCalls = enabledCalls
					}
					for sample.AdvanceCalls < maxCalls && c.Rendering != nil {
						step()
						if !disabled && sample.Shots > 0 {
							break
						}
					}
					if disabled {
						if sample.Shots != 0 {
							t.Fatal("disabled hostile release", sample)
						}
					} else {
						if sample.Shots == 0 {
							t.Fatal("no natural family release", name)
						}
						if name == "guided" && sample.PlayerGuidedShots == 0 {
							t.Fatal("R3 baseline did not target player", sample)
						}
						enabledCalls = sample.AdvanceCalls
					}
					if output := os.Getenv("GS2000_R23_OUTPUT"); output != "" {
						os.MkdirAll(output, 0700)
						b, _ := json.MarshalIndent(sample, "", "  ")
						os.WriteFile(filepath.Join(output, fmt.Sprintf("%s-disabled-%v.json", name, disabled)), b, 0600)
					}
					// One complete air encounter and the player-guided baseline traverse normal
					// end/debrief/final acknowledgment, restart, strict playback and Resume.
					if !disabled && (name == "fixed" || name == "guided") {
						for n := 0; n < 200 && c.Rendering != nil; n++ {
							step()
						}
						finishAndReplay(t, c, before, name)
					}
				})
			}
		})
	}
}

func finishAndReplay(t *testing.T, c *FrontEnd, before []byte, name string, profiles ...string) {
	t.Helper()
	p := c.Rendering
	sortieID := c.Mission.ResultLaunch.Start.ID
	if c.Rendering != nil {
		editorAction(t, c, Actions{EndMission: true})
		editorAction(t, c, Actions{ConfirmQuit: true})
	}
	if c.Screen != ScreenDebrief {
		t.Fatal("normal ending", c.Screen, c.Message)
	}
	if c.Debrief.Pending == nil || c.Debrief.Pending.Proposal.Result == nil {
		t.Fatal("no actual proposed result")
	}
	staged, _ := json.Marshal(c.Store.State().SelectedPilot())
	if !bytes.Equal(before, staged) {
		t.Fatal("career before acknowledgment")
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
	expected := replay.R23AuthoredCompatibility
	if len(profiles) > 0 {
		expected = profiles[0]
	}
	if f.Header.Simulation != expected {
		t.Fatal("interpretation", f.Header.Simulation)
	}
	after, _ := json.Marshal(c.Store.State())
	afterPilot, _ := json.Marshal(c.Store.State().SelectedPilot())
	if bytes.Equal(before, afterPilot) {
		t.Fatal("no durable actual result")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, filepath.Join(t.TempDir(), "no-original"))
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.CommitSortie(sortieID); err != nil {
		t.Fatal(err)
	}
	restarted, _ := json.Marshal(reopened.State())
	if !bytes.Equal(after, restarted) {
		t.Fatal("restart/duplicate result changed")
	}
	opt := c.Runtime
	opt.DisableEnemyAI = true
	opt.DisableWingmenAI = p.wingmenAutonomy
	q, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal("strict", err)
		}
	}
	practice, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 80 && !practice.Done; n++ {
		if err = practice.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if err = practice.Resume(); err != nil {
		t.Fatal(err)
	}
	if practice.Engine.Store != nil || practice.Engine.Mission.ResultLaunch != nil {
		t.Fatal("Resume acquired career")
	}
	if err = practice.AdvancePractice(.1); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(after, unchanged) {
		t.Fatal("Resume changed career")
	}
	if output := os.Getenv("GS2000_R23_OUTPUT"); output != "" {
		b, err := replay.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, name+"-complete.g2r"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("actual result score%d film%s operations%d strict/restart/isolated Resume pass", f.End.Score, f.Header.ID, len(f.Operations))
}

func TestGeneratedEuro9Su25ColdActivation(t *testing.T) {
	base := os.Getenv("GS2000_R23_WITNESSES")
	if base == "" {
		t.Skip("R23 fresh supplied pilot required")
	}
	c := naturalControllerMode(t, filepath.Join(base, "fixed", "try01", "app"), false, false, 1)
	defer c.AbandonFilmRecording()
	p := c.Rendering
	if c.Mission.Data.World != "EURO9.WRL" {
		t.Fatal("generated world", c.Mission.Data.World)
	}
	row := 65
	e := p.Initialized.Entities[row]
	if e.Kind != 1 || e.ModelID != 86 || e.Object.Formation != 130 || e.Pose.H0 != 67432 || e.Pose.H1 != 514300 || e.Participation != 0 {
		t.Fatal("retained Step52 generated constructor", e)
	}
	retained := false
	for _, instance := range p.instances {
		if instance.ID == e.ID {
			retained = true
		}
	}
	if !retained {
		t.Fatal("cold renderer header omitted")
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	if err := c.advanceFlight(.1); err != nil {
		t.Fatal(err)
	}
	first := p.Initialized.Entities[row]
	if first.Participation != 1 || first.Pose.H0 != e.Pose.H0 || first.Pose.H1 != e.Pose.H1 || first.Pose.Height != 1024 || first.RawMotion != 400 {
		t.Fatal("first visit is not cold source P0", first)
	}
	if err := c.advanceFlight(.1); err != nil {
		t.Fatal(err)
	}
	second := p.Initialized.Entities[row]
	if second.Pose.H0 == first.Pose.H0 && second.Pose.H1 == first.Pose.H1 {
		t.Fatal("second visit did not move", second)
	}
	t.Logf("normal generated EURO9 seed1 previous0 F130 row65 cold%+v first%+v second%+v", e.Pose, first.Pose, second.Pose)
}
