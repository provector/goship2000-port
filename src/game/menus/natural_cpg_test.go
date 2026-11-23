package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// r3NaturalCPGController copies a normally saved authored encounter and uses
// the real Settings selector/persistence/APU path. It does not set runtime gates.
func naturalCPGController(t *testing.T, choice int, profiles ...string) *FrontEnd {
	t.Helper()
	assets, base := os.Getenv("GS2000_TEST_MISSIONS"), os.Getenv("GS2000_R23_WITNESSES")
	if assets == "" || base == "" {
		t.Skip("prepared R231 assets and retained authored encounter required")
	}
	fixture := filepath.Join(base, "guided", "try02", "app")
	a, err := assetdata.LoadFrontEndAssets(assets)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	var copies []map[string]any
	err = filepath.WalkDir(filepath.Join(fixture, "saves"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Join(fixture, "saves"), path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, "saves", rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("encounter symlink")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(b) > 1<<20 {
			return fmt.Errorf("unbounded encounter member")
		}
		if err = os.WriteFile(target, b, 0600); err != nil {
			return err
		}
		copies = append(copies, map[string]any{"source": path, "sha256": fmt.Sprintf("%x", sha256.Sum256(b)), "operation": "exact copy into isolated test profile before normal UI/result writes", "output": target, "output_sha256": fmt.Sprintf("%x", sha256.Sum256(b))})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if output := os.Getenv("GS2000_R3_OUTPUT"); output != "" {
		b, _ := json.MarshalIndent(copies, "", "  ")
		if err = os.WriteFile(filepath.Join(output, fmt.Sprintf("r3-guided-choice%d-copy-provenance.json", choice)), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := gamestate.OpenSaveStore(filepath.Join(root, "saves"), filepath.Join(root, "no-original"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	// Pin this retained R3 witness to its original interpretation. R4's
	// inheritance fixture explicitly requests the new launch profile below;
	// historical R3 expectations/films are not retagged as a newer simulation.
	profile := replay.R3AuthoredCompatibility
	if len(profiles) != 0 {
		profile = profiles[0]
	}
	c.Runtime.DisableEnemyAI, c.Runtime.DisableWingmenAI = false, true
	c.FilmDirectory, c.MissionDirectory = filepath.Join(root, "films"), filepath.Join(root, "missions")
	files, err := filepath.Glob(filepath.Join(fixture, "missions", "*.g2m"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	id := strings.TrimSuffix(filepath.Base(files[0]), ".g2m")
	doc, _, err := (gamestate.AuthoredStore{Directory: filepath.Join(fixture, "missions")}).Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.beginAuthoredMission(doc); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
		if err = c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
	missionClick(t, c, 3)
	for n := 0; n < 3 && c.Mission.Settings.CPGControls != choice; n++ {
		missionClick(t, c, 1)
	}
	if c.Mission.Settings.CPGControls != choice {
		t.Fatal("CP/G selector", choice, c.Mission.Settings)
	}
	editorAction(t, c, Actions{Cancel: true})
	if c.Screen != ScreenBriefing {
		t.Fatal("settings return", c.Screen)
	}
	reopened, err := gamestate.OpenSaveStore(store.Directory, filepath.Join(root, "no-original"))
	if err != nil || reopened.State().SelectedPilot().Preflight.CPGControls != choice {
		t.Fatal("persisted CP/G choice", choice, err)
	}
	// A nonempty interpretation is otherwise the playback path and requires
	// its frozen ENEM owner. Freeze the actual saved settings/world exactly as
	// the ordinary launch does; only the test's historical version is selected.
	switches, err := gamestate.LaunchSwitches(c.Mission.Data, c.Mission.Settings, a.Missions.Generation.Tables.Initialization.CPGMapping)
	if err != nil {
		t.Fatal(err)
	}
	c.enemyLaunchRules, err = freezePreparedEnemyRules(a.Missions, c.Mission.Data, switches, c.Runtime.DisableEnemyAI)
	if err != nil {
		t.Fatal(err)
	}
	c.simulationProfile = profile
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	p := c.Rendering
	mask := []uint16{0x18, 8, 0}[choice]
	if !p.copilotMechanics || !p.threatFeedbackMechanics || p.Initialized.Switches&0x18 != mask || p.Initialized.EnemyRules == nil || p.Initialized.EnemyRules.Disabled || p.Flight.Defenses.R3 == nil {
		t.Fatal("ordinary R3 launch", choice, p.Initialized.Switches, p.copilotMechanics)
	}
	t.Cleanup(c.AbandonFilmRecording)
	return c
}

type naturalTick struct {
	Step                    uint64          `json:"step"`
	Phase                   uint16          `json:"phase"`
	Timing                  flight.Timing   `json:"recorded_timing"`
	RNG                     uint16          `json:"rng"`
	Pose                    flight.Pose     `json:"pose"`
	Defenses                flight.Defenses `json:"defenses"`
	AEB                     uint16          `json:"aeb_after_late_consumer"`
	Target                  uint16          `json:"target"`
	Symbol                  int             `json:"target_symbol"`
	Station                 int             `json:"station"`
	SelectedWeaponRawID     int             `json:"selected_weapon_raw_id"`
	SelectedWeaponFlags     uint16          `json:"selected_weapon_flags"`
	ManualTrigger           bool            `json:"manual_trigger"`
	Damage                  [12]byte        `json:"player_damage"`
	TargetKind, TargetP     byte
	AttackerKind, AttackerP byte
	NextSlot, LastSlot      uint16
	IncomingAimKnown        bool
	IncomingAimPose         any
	Ammo                    [4]int16                `json:"ammunition"`
	Events                  []combat.Event          `json:"events,omitempty"`
	AutoDefense             []CopilotDefenseWitness `json:"automatic_defense,omitempty"`
	Active                  []naturalProjectile     `json:"active_projectiles,omitempty"`
}

type naturalProjectile struct {
	Slot       int
	Projectile combat.Projectile
}

type naturalCase struct {
	Choice                                              int    `json:"ui_choice"`
	Mask                                                uint16 `json:"frozen_mask"`
	FirstDefense                                        uint64 `json:"first_defense_step,omitempty"`
	FirstWeapon                                         uint64 `json:"first_weapon_step,omitempty"`
	FirstHostile                                        uint64 `json:"first_hostile_step,omitempty"`
	HostileShots, PlayerShots, Defeats, Misses, Impacts int
	GuidanceDisabled                                    int
	FirstGuidanceDisabled                               uint64 `json:"first_guidance_disabled_step,omitempty"`
	FirstGuidanceOperation                              int
	FirstIRFallback                                     uint64 `json:"first_ir_fallback_step,omitempty"`
	FirstDefenseOperation, FirstIRFallbackOperation     int
	FirstActionOperation                                int           `json:"first_action_operation,omitempty"`
	Ticks                                               []naturalTick `json:"ticks"`
}

func TestNaturalCPGChoices(t *testing.T) {
	output := os.Getenv("GS2000_R3_OUTPUT")
	if output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GS2000_R23_OUTPUT", output)
	}
	for _, choice := range []int{2, 1, 0} {
		t.Run(fmt.Sprint("choice", choice), func(t *testing.T) {
			c := naturalCPGController(t, choice)
			p, r := c.Rendering, c.Rendering.Flight
			before, _ := json.Marshal(c.Store.State().SelectedPilot())
			sample := naturalCase{Choice: choice, Mask: p.Initialized.Switches & 0x18}
			name := fmt.Sprintf("r3-guided-choice%d", choice)
			defer func() {
				if output != "" {
					b, _ := json.MarshalIndent(sample, "", "  ")
					if err := os.WriteFile(filepath.Join(output, name+".json"), b, 0600); err != nil {
						t.Error(err)
					}
				}
			}()
			defenseSeen := 0
			step := func() {
				t.Helper()
				prior := r.Steps
				if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
					t.Fatal(err)
				}
				if r.Steps != prior+1 {
					t.Fatal("one reached logical tick per sample", prior, r.Steps)
				}
				if r.commandInput.Trigger {
					t.Fatal("natural fixture never presses trigger")
				}
				v := naturalTick{Step: r.Steps, Phase: r.ElapsedDA, RNG: p.Initialized.RandomState, Pose: r.State.Pose, Defenses: r.Defenses, AEB: r.Combat.IncomingThreat, Target: r.Avionics.Target, Symbol: p.Cockpit.TargetSymbol, Station: p.Initialized.SelectedStation}
				last := p.filmRecorder.lastOperation
				var recorded filmStep
				if last.Kind != "step" || json.Unmarshal(last.Payload, &recorded) != nil {
					t.Fatal("logical film timing absent")
				}
				v.Timing, v.ManualTrigger, v.Damage = recorded.Timing, recorded.Input.Trigger, gamestate.PlayerDamage12(&r.Systems)
				station := p.Initialized.Stores.Ships[0].Stations[v.Station]
				v.SelectedWeaponRawID = station.RawWSHDID
				for _, w := range p.Initialized.Stores.Definitions {
					if w.CompactID == station.CompactID {
						v.SelectedWeaponFlags = binary.LittleEndian.Uint16(w.Raw[:])
					}
				}
				fs := r.Combat.FilmState()
				v.NextSlot, v.LastSlot, v.IncomingAimKnown, v.IncomingAimPose = fs.NextSlot, fs.LastSlot, fs.IncomingAimKnown, fs.IncomingAimPose
				if v.Target > 0 {
					target := p.Initialized.Entities[int(v.Target)-1]
					v.TargetKind, v.TargetP = target.Kind, target.Participation
				}
				v.AttackerKind, v.AttackerP = p.Initialized.Entities[20].Kind, p.Initialized.Entities[20].Participation
				// Deep-copy pointer-bearing defense ownership before later ticks mutate it.
				if r.Defenses.R3 != nil {
					own := *r.Defenses.R3
					v.Defenses.R3 = &own
					if own.ManualHeld != [2]bool{} {
						t.Fatal("natural comparison never holds manual jammer")
					}
				}
				for i, s := range p.Initialized.Stores.Ships[0].Stations {
					v.Ammo[i] = s.Ammunition
				}
				v.Events = append(v.Events, r.CombatEvents...)
				for _, e := range r.CombatEvents {
					if e.Kind == "launch" && e.Actor == 0 {
						if v.ManualTrigger || !v.Timing.E0 || v.Phase&7 != 0 || v.Target == 0 || v.Symbol != 1 || v.TargetKind&0x30 != 0 || v.TargetP != 1 {
							t.Fatal("actual positive source predicates", v)
						}
						sample.PlayerShots++
						if sample.FirstWeapon == 0 {
							sample.FirstWeapon = r.Steps
						}
					}
					if e.Kind == "launch" && e.Actor == 20 && e.Target == 0 {
						sample.HostileShots++
						if sample.FirstHostile == 0 {
							sample.FirstHostile = r.Steps
						}
					}
					if e.Kind == "hostile-defended" {
						sample.Defeats++
					}
					if e.Kind == "hostile-guidance-defeated" {
						sample.GuidanceDisabled++
						if sample.FirstGuidanceDisabled == 0 {
							sample.FirstGuidanceDisabled = r.Steps
							sample.FirstGuidanceOperation = p.filmRecorder.operationCount()
						}
					}
					if e.Kind == "hostile-miss" {
						sample.Misses++
					}
					if e.Kind == "hostile-impact" {
						sample.Impacts++
					}
				}
				r.CombatEvents = nil
				if len(r.CopilotDefense) > defenseSeen {
					v.AutoDefense = append(v.AutoDefense, r.CopilotDefense[defenseSeen:]...)
					if sample.FirstDefense == 0 {
						sample.FirstDefense = v.AutoDefense[0].Step
						sample.FirstDefenseOperation = p.filmRecorder.operationCount()
					}
					defenseSeen = len(r.CopilotDefense)
				}
				for slot, s := range r.Combat.Slots {
					if s.Lifetime != 0 {
						v.Active = append(v.Active, naturalProjectile{slot, s})
					}
				}
				sample.Ticks = append(sample.Ticks, v)
				if sample.FirstActionOperation == 0 && (sample.FirstDefense != 0 || sample.FirstWeapon != 0) {
					sample.FirstActionOperation = p.filmRecorder.operationCount()
				}
				if sample.FirstIRFallback == 0 && r.Defenses.IRActive != 0 && r.Defenses.R3.Automatic[1] {
					sample.FirstIRFallback = r.Steps
					sample.FirstIRFallbackOperation = p.filmRecorder.operationCount()
				}
				if p.Error != nil || r.Failure != "" {
					t.Fatal(p.Error, r.Failure)
				}
			}
			c.Handle(Actions{FlightCommands: []int{2}})
			for n := 0; n < 40; n++ {
				step()
			}
			editorAction(t, c, Actions{GPSOpen: true})
			for n := 0; n < 4; n++ {
				editorAction(t, c, Actions{GPSKey: "n"})
				editorAction(t, c, Actions{GPSKey: "h"})
			}
			editorAction(t, c, Actions{ViewKey: 1})
			for n := 0; n < 40; n++ {
				c.Handle(Actions{FlightCommands: []int{0}})
				step()
			}
			c.Handle(Actions{})
			c.Handle(Actions{FlightCommands: []int{10}})
			c.Handle(Actions{})
			for n := 0; n < 1600 && c.Rendering != nil; n++ {
				step()
			}
			t.Logf("choice%d mask%02x hostile%d own%d autoDefenseStep%d autoWeaponStep%d contactDefeats%d guidanceDisabled%d terminalMisses%d impacts%d IRfallbackStep%d", choice, sample.Mask, sample.HostileShots, sample.PlayerShots, sample.FirstDefense, sample.FirstWeapon, sample.Defeats, sample.GuidanceDisabled, sample.Misses, sample.Impacts, sample.FirstIRFallback)
			if choice == 2 && (sample.FirstDefense != 0 || sample.FirstWeapon != 0) {
				t.Fatal("None automated")
			}
			if choice == 1 && sample.FirstWeapon != 0 {
				t.Fatal("CM fired weapons")
			}
			if sample.HostileShots == 0 {
				t.Fatal("no real Infantry20 shot")
			}
			if choice == 1 && sample.FirstDefense == 0 {
				t.Fatal("no natural CM action within bound")
			}
			if choice == 1 && (sample.FirstIRFallback == 0 || sample.GuidanceDisabled == 0) {
				t.Fatal("no depleted-stock natural IR guidance disable", sample.FirstIRFallback, sample.GuidanceDisabled)
			}
			if choice == 0 && sample.FirstWeapon == 0 {
				t.Fatal("no natural weapon action within bound")
			}
			finishAndReplay(t, c, before, name, replay.R3AuthoredCompatibility)
			if sample.FirstActionOperation > 0 {
				naturalAfterActionResume(t, c, sample.FirstActionOperation)
			}
			if sample.FirstDefenseOperation > 0 && sample.FirstDefenseOperation != sample.FirstActionOperation {
				naturalAfterActionResume(t, c, sample.FirstDefenseOperation)
			}
			if sample.FirstIRFallbackOperation > 0 {
				naturalAfterActionResume(t, c, sample.FirstIRFallbackOperation)
			}
			if sample.FirstGuidanceOperation > 0 {
				naturalAfterActionResume(t, c, sample.FirstGuidanceOperation)
			}
		})
	}
}

func naturalAfterActionResume(t *testing.T, c *FrontEnd, operation int) {
	t.Helper()
	f, err := c.Library.Store.Load(c.Library.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(c.Store.State())
	opt := c.Runtime
	opt.DisableEnemyAI = true
	opt.DisableWingmenAI = false
	q, err := NewFilmPlayback(c.Office.Assets, f, opt)
	if err != nil {
		t.Fatal(err)
	}
	for q.Cursor < operation && !q.Done {
		if err = q.Step(); err != nil {
			t.Fatal("after-action strict", err)
		}
	}
	if q.Done {
		t.Fatal("after-action boundary reached end")
	}
	p := q.Preview()
	defenses, _ := json.Marshal(p.Flight.Defenses)
	if err = q.Resume(); err != nil {
		t.Fatal(err)
	}
	restored, _ := json.Marshal(p.Flight.Defenses)
	if !bytes.Equal(defenses, restored) || q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil {
		t.Fatal("after-action Resume state/isolation")
	}
	if err = q.AdvancePractice(.1); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(after, unchanged) {
		t.Fatal("after-action practice changed career")
	}
	t.Logf("after-action strict operation%d step%d then isolated Resume passed", operation, p.Flight.Steps)
}
