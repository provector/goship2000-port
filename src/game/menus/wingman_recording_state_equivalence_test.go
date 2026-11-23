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
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
)

// A saved mission and fresh supplied Captain expose the normal BEGIN/preflight
// route. No live contact, projectile, damage, return flag or result is supplied.
func naturalEncounterNaturalController(t *testing.T, disabled bool) *FrontEnd {
	return naturalEncounterNaturalControllerProfile(t, disabled, replay.R22AuthoredCompatibility)
}

func naturalEncounterNaturalControllerProfile(t *testing.T, disabled bool, profile string) *FrontEnd {
	t.Helper()
	assets, fixture := os.Getenv("GS2000_TEST_MISSIONS"), os.Getenv("GS2000_R22_WITNESS")
	if assets == "" || fixture == "" {
		t.Skip("R22 prepared assets and saved authored encounter required")
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
	t.Cleanup(c.AbandonFilmRecording)
	c.simulationProfile = profile // Empty is the current ordinary launch; old fixtures are explicit.
	c.Runtime.DisableEnemyAI = disabled
	c.Runtime.DisableWingmenAI = false // Real acquisition supplies the hostile target; W below holds fire.
	c.FilmDirectory = filepath.Join(root, "films")
	c.MissionDirectory = filepath.Join(root, "missions")
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
	for i := 0; i < 100 && c.Screen != ScreenBriefing; i++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
	if profile != "" {
		// Preserve the retained R22 witness interpretation after new ordinary
		// launches advance to R221. The original test's exact header stays R22.
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
	if !c.Rendering.wingmanDamageMechanics || c.Rendering.Initialized.EnemyRules == nil || c.Rendering.Initialized.EnemyRules.Disabled != disabled {
		t.Fatal("frozen R22 rules missing")
	}
	return c
}

func TestWingmanRecordingStateEquivalence(t *testing.T) {
	var want map[string]string
	var firstSteps uint64
	for _, record := range []bool{true, false} {
		t.Run(fmt.Sprint(record), func(t *testing.T) {
			c := naturalEncounterNaturalController(t, false)
			p, r := c.Rendering, c.Rendering.Flight
			defer c.AbandonFilmRecording()
			if !record {
				c.AbandonFilmRecording()
			}
			gun := -1
			for i, e := range p.Initialized.Entities {
				if e.Kind == 6 && e.Object.Raw[11] == 0x55 && e.DynamicRaw[2] == 0 {
					gun = i
					break
				}
			}
			if gun < 0 {
				t.Fatal("gun missing")
			}
			c.Handle(Actions{})
			c.Handle(Actions{FlightCommands: []int{2}})
			for i := 0; i < 40; i++ {
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
			}
			issueGunRoute(t, c, gun)
			for i := 0; i < 3600; i++ {
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
				if p.Error != nil || r.Failure != "" {
					t.Fatal(p.Error, r.Failure)
				}
				if p.Initialized.Stores.Damage12(1) != ([12]byte{}) {
					break
				}
			}
			if p.Initialized.Stores.Damage12(1) == ([12]byte{}) {
				t.Fatal("no real NPC damage")
			}
			sample := p.FilmCheckpoint()
			if record {
				want = sample
				firstSteps = r.Steps
			} else if firstSteps != r.Steps || !reflect.DeepEqual(want, sample) {
				t.Fatalf("recording changes authoritative state: steps%d/%d group%s", firstSteps, r.Steps, compareFilmCheckpoint(want, sample))
			}
		})
	}
}

func issueGunRoute(t *testing.T, c *FrontEnd, gun int) {
	t.Helper()
	p := c.Rendering
	act := func(a Actions) {
		t.Helper()
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	act(Actions{})
	act(Actions{GPSOpen: true})
	act(Actions{})
	act(Actions{GPSKey: "n"})
	if p.GPS == nil || p.GPS.Selected != 1 {
		t.Fatal("selected wingman", p.GPS)
	}
	act(Actions{GPSKey: "w"}) // Hold weapons, preserving real contact acquisition.
	act(Actions{GPSKey: "f"})
	e := p.Initialized.Entities[gun]
	x, y := navigation.HighPixel(navigation.Point{H0: e.Pose.H0 + 2048, H1: e.Pose.H1})
	act(Actions{Inside: true, X: x, Y: y, Selector1: true})
	act(Actions{})
	act(Actions{GPSKey: "f"})
	act(Actions{GPSKey: "a"}) // Medium192: sensing envelope exceeds route arrival radius.
	act(Actions{ViewKey: 1})
	act(Actions{})
	t.Logf("ordinary path actor1 %v order%d permission%d", p.Initialized.Entities[1].ControlRaw[10:40], p.Initialized.Entities[1].ControlRaw[1], p.Initialized.Entities[1].ControlRaw[4])
}

func TestNaturalWingmanEncounterResultReplay(t *testing.T) {
	naturalWingmanEncounter(t, false)
}

func TestOrdinaryLaunchWingmanResultReplay(t *testing.T) {
	naturalWingmanEncounter(t, true)
}

func naturalWingmanEncounter(t *testing.T, current bool, profiles ...string) {
	output := os.Getenv("GS2000_R22_OUTPUT")
	profile, want := replay.R22AuthoredCompatibility, replay.R22AuthoredCompatibility
	if current {
		profile, want, output = replay.R221AuthoredCompatibility, replay.R221AuthoredCompatibility, os.Getenv("GS2000_R221_OUTPUT")
	}
	if len(profiles) > 0 {
		profile, want, output = profiles[0], profiles[0], os.Getenv("GS2000_R231_OUTPUT")
	}
	c := naturalEncounterNaturalControllerProfile(t, false, profile)
	p, r := c.Rendering, c.Rendering.Flight
	before, _ := json.Marshal(c.Store.State().SelectedPilot())
	sortieID := c.Mission.ResultLaunch.Start.ID
	gun := -1
	for i, e := range p.Initialized.Entities {
		if e.Kind == 6 && e.Object.Raw[11] == 0x55 && e.DynamicRaw[2] == 0 {
			gun = i
			break
		}
	}
	if gun < 0 {
		t.Fatal("ordinary Fixed Gun absent")
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	for i := 0; i < 40; i++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
	}
	issueGunRoute(t, c, gun)
	shots, victim, steps := 0, -1, 0
	var hit [12]byte
	for ; steps < 3600 && c.Rendering != nil; steps++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
		if p.Error != nil || r.Failure != "" {
			t.Fatal("natural encounter stopped", p.Error, r.Failure)
		}
		for _, e := range r.CombatEvents {
			if e.Kind == "launch" && e.Actor == gun && e.Slot >= 20 && e.Target >= 1 && e.Target <= 4 {
				shots++
			}
		}
		r.CombatEvents = nil // Bounded diagnostic retention, excluded from checkpoints.
		for actor := 1; actor < 5; actor++ {
			d := p.Initialized.Stores.Damage12(actor)
			if d != ([12]byte{}) {
				victim, hit = actor, d
				break
			}
		}
		if victim >= 0 {
			break
		}
	}
	if shots == 0 || victim < 1 {
		t.Fatalf("no natural wingman damage shots=%d victim=%d calls=%d guncontact=%+v follower=%+v", shots, victim, steps, r.Avionics.Contacts[gun], p.Initialized.Entities[1])
	}
	t.Logf("real shooter%d victim%d slot%d kind%02x damage%v atstep%d", gun, victim, victim+1, p.Initialized.Entities[victim].Kind, hit, r.Steps)
	// Recall through ordinary GPS. The live damaged actor owns return/loss.
	c.Handle(Actions{})
	c.Handle(Actions{GPSOpen: true})
	c.Handle(Actions{})
	c.Handle(Actions{GPSKey: "n"})
	c.Handle(Actions{GPSKey: "b"})
	c.Handle(Actions{ViewKey: 1})
	c.Handle(Actions{})
	for i := 0; i < 1800 && c.Rendering != nil; i++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
		if p.Error != nil || r.Failure != "" {
			t.Fatal(p.Error, r.Failure)
		}
	}
	if c.Rendering != nil {
		editorAction(t, c, Actions{EndMission: true})
		editorAction(t, c, Actions{ConfirmQuit: true})
	}
	if c.Screen != ScreenDebrief {
		t.Fatal("normal ending", c.Screen, c.Message)
	}
	proposal := c.Debrief.Pending.Proposal
	if proposal.Result == nil || proposal.Facts.ResultContext == nil {
		t.Fatal("actual crew result missing")
	}
	returned := proposal.Facts.ResultContext.Actors[victim]
	finalDamage := p.Initialized.Stores.Damage12(victim)
	health := gamestate.EncodedDamageHealth(finalDamage)
	distance := gamestate.SourceServiceDistance(c.Mission.Data.Objectives["base"].Point, flight.Pose{H0: returned.Pose.H0, H1: returned.Pose.H1, Height: int16(returned.Pose.Height), Angles: returned.Pose.Angles})
	if returned.Damage != finalDamage || proposal.Result.NormalizedFlags[victim]&0x80 == 0 || returned.Kind == 0x1c || distance > 768 {
		t.Fatalf("actual safe return/full damage absent: kind%02x control%d distance%d flags%02x damage%v want%v", returned.Kind, returned.Control, distance, proposal.Result.NormalizedFlags[victim], returned.Damage, finalDamage)
	}
	crewDefense := -1
	for _, metric := range proposal.Result.Crew {
		if metric.Actor == victim {
			crewDefense = metric.Defense
		}
	}
	if crewDefense != int(byte(health))*100/255 {
		t.Fatal("actual crew damage metric", health, crewDefense)
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
	if f.Header.Simulation != want {
		t.Fatal(f.Header.Simulation)
	}
	after, _ := json.Marshal(c.Store.State())
	afterPilot, _ := json.Marshal(c.Store.State().SelectedPilot())
	if bytes.Equal(before, afterPilot) {
		t.Fatal("no durable result")
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
		t.Fatal("restart or duplicate acknowledgment changed result")
	}
	opt := c.Runtime
	opt.DisableEnemyAI = true
	opt.DisableWingmenAI = current
	q, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal(err)
		}
	}
	practice, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for !practice.Done && practice.Preview().Initialized.Stores.Damage12(victim) == ([12]byte{}) {
		if err = practice.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if practice.Done {
		t.Fatal("no reached wingman damage before Resume")
	}
	resumeDamage := practice.Preview().Initialized.Stores.Damage12(victim)
	if err = practice.Resume(); err != nil {
		t.Fatal(err)
	}
	if practice.Engine.Store != nil {
		t.Fatal("Resume owns career")
	}
	if practice.Preview().Initialized.Stores.Damage12(victim) != resumeDamage {
		t.Fatal("Resume lost full NPC damage")
	}
	if err = practice.AdvancePractice(.1); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(after, unchanged) {
		t.Fatal("practice changed career")
	}
	if output != "" {
		if err = os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		b, err := replay.Marshal(f)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, "natural-wingman.g2r"), b, 0600); err != nil {
			t.Fatal(err)
		}
		note, _ := json.MarshalIndent(map[string]any{"actor": gun, "victim": victim, "callsign": returned.Callsign, "damage_after_first_hit": hit, "final_damage": finalDamage, "health": health, "crew_defense": crewDefense, "return_pose": returned.Pose, "return_kind": returned.Kind, "return_control": returned.Control, "return_base_distance": distance, "normalized_flags": proposal.Result.NormalizedFlags[victim], "shots": shots, "steps_to_hit": steps, "score": f.End.Score, "film_id": f.Header.ID, "scope": "Native automated normal saved mission/controls; no supplied live pose/projectile/damage/return/result. Connected OS input and human observation are separate."}, "", "  ")
		if err = os.WriteFile(filepath.Join(output, "natural-wingman-result.json"), note, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
