package menus

import (
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type radarTick struct {
	Defeated                              *combat.Projectile `json:",omitempty"`
	Step                                  uint64
	Phase                                 uint16
	RNG                                   uint16
	RNGPrevious                           uint16
	RadarHistoryBefore, RadarHistoryAfter uint16
	Pose                                  flight.Pose
	Threat                                uint16
	Defenses                              flight.Defenses
	Events                                []combat.Event          `json:",omitempty"`
	Auto                                  []CopilotDefenseWitness `json:",omitempty"`
	Active                                []combat.Projectile     `json:",omitempty"`
}

type radarCase struct {
	Choice                                  int
	World                                   string
	ObjectHash                              string
	Launches, RadarLaunches, BypassLaunches int
	FirstLaunch, FirstAuto                  uint64
	FirstFallback, FirstGuidanceDefeat      uint64
	GuidanceDefeats                         int
	Ticks                                   []radarTick
}

// TestR3NaturalRadarDefense uses only a fresh normally saved document, the real
// Settings/APU path and ordinary rotor/collective/GPS/autopilot input. No actor,
// projectile, target, contact, defense stock, damage or RNG value is supplied.
func TestNaturalRadarDefense(t *testing.T) {
	fixture := os.Getenv("GS2000_R3_RADAR_FIXTURE")
	if fixture == "" {
		t.Skip("fresh saved radar authoring fixture required")
	}
	// Reuse the actual Settings-selector helper without altering its IR witness.
	base := t.TempDir()
	target := filepath.Join(base, "guided", "try02", "app")
	if err := filepath.WalkDir(fixture, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fixture, path)
		if err != nil {
			return err
		}
		to := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0700)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("radar fixture symlink")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(b) > 1<<20 {
			return fmt.Errorf("radar fixture member exceeds bound")
		}
		return os.WriteFile(to, b, 0600)
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GS2000_R23_WITNESSES", base)
	output := os.Getenv("GS2000_R3_RADAR_OUTPUT")
	if output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var controls [2]radarCase
	for i, choice := range []int{2, 1} {
		t.Run(fmt.Sprintf("choice%d", choice), func(t *testing.T) {
			c := naturalCPGController(t, choice)
			p, r := c.Rendering, c.Rendering.Flight
			if p.Initialized.Entities[20].Object.Raw[12] != 55 || p.Initialized.Entities[20].Object.Raw[11] != 0xcc {
				t.Fatal("saved SA-6 actor source fields differ", p.Initialized.Entities[20])
			}
			sample := radarCase{Choice: choice, World: p.Initialized.World, ObjectHash: c.Mission.Data.ObjectHash}
			defer func() {
				controls[i] = sample
				if output != "" {
					b, _ := json.MarshalIndent(sample, "", "  ")
					if err := os.WriteFile(filepath.Join(output, fmt.Sprintf("choice%d.json", choice)), b, 0600); err != nil {
						t.Error(err)
					}
				}
			}()
			autoSeen := 0
			step := func() {
				t.Helper()
				prior := r.Steps
				rngPrior, historyPrior := p.Initialized.RandomState, r.Combat.IncomingRadarHistory
				if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
					t.Fatal(err)
				}
				if r.Steps != prior+1 {
					t.Fatal("expected one logical visit", prior, r.Steps)
				}
				if p.Error != nil || r.Failure != "" {
					t.Fatal(p.Error, r.Failure)
				}
				v := radarTick{Step: r.Steps, Phase: r.ElapsedDA, RNG: p.Initialized.RandomState, RNGPrevious: rngPrior, RadarHistoryBefore: historyPrior, RadarHistoryAfter: r.Combat.IncomingRadarHistory, Pose: r.State.Pose, Threat: r.Combat.IncomingThreat, Defenses: r.Defenses}
				v.Events = append(v.Events, r.CombatEvents...)
				for _, ev := range r.CombatEvents {
					if ev.Kind == "hostile-guidance-defeated" && ev.Actor == 20 && ev.Target == 0 {
						slot := r.Combat.Slots[ev.Slot]
						v.Defeated = &slot
						if slot.Parameter != 0 || slot.Flags&3 != 3 {
							t.Fatal("guidance-disable diagnostic lacks actual source slot writes", slot)
						}
						sample.GuidanceDefeats++
						if sample.FirstGuidanceDefeat == 0 {
							sample.FirstGuidanceDefeat = r.Steps
						}
					}
					if ev.Kind == "launch" && ev.Actor == 20 && ev.Target == 0 {
						sample.Launches++
						if sample.FirstLaunch == 0 {
							sample.FirstLaunch = r.Steps
						}
						slot := r.Combat.Slots[ev.Slot]
						if slot.Flags&0x40 != 0 {
							sample.RadarLaunches++
						}
						if slot.Flags&0x8000 != 0 {
							sample.BypassLaunches++
						}
					}
				}
				r.CombatEvents = nil
				if len(r.CopilotDefense) > autoSeen {
					v.Auto = append(v.Auto, r.CopilotDefense[autoSeen:]...)
					autoSeen = len(r.CopilotDefense)
					for _, ev := range v.Auto {
						if ev.Threats&0x40 != 0 && sample.FirstAuto == 0 {
							sample.FirstAuto = ev.Step
						}
						if ev.Threats&0x40 != 0 && r.Defenses.R3.Automatic[0] && r.Defenses.RadarSignature != 0 && sample.FirstFallback == 0 {
							sample.FirstFallback = ev.Step
						}
					}
				}
				for _, s := range r.Combat.Slots {
					if s.Lifetime != 0 {
						v.Active = append(v.Active, s)
					}
				}
				sample.Ticks = append(sample.Ticks, v)
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
			for n := 0; n < 2400 && c.Rendering != nil; n++ {
				step()
			}
			t.Logf("choice%d actor20 radar launches%d/%d bypass%d firstlaunch%d firstauto%d chaff%d flares%d pose%+v", choice, sample.RadarLaunches, sample.Launches, sample.BypassLaunches, sample.FirstLaunch, sample.FirstAuto, r.Defenses.Chaff, r.Defenses.Flares, r.State.Pose)
			if sample.RadarLaunches == 0 {
				t.Fatal("no naturally constructed radar-guided launch")
			}
			if choice == 2 && sample.FirstAuto != 0 {
				t.Fatal("None automated")
			}
			if choice == 1 && (sample.FirstAuto == 0 || r.Defenses.Chaff >= 30) {
				t.Fatal("no positive chaff response from physical AEB40")
			}
			if choice == 1 && (sample.FirstFallback == 0 || sample.GuidanceDefeats == 0) {
				t.Fatal("missing natural radar fallback/guidance-disable cause", sample.FirstFallback, sample.GuidanceDefeats)
			}
		})
	}
	if controls[0].ObjectHash != controls[1].ObjectHash {
		t.Fatal("control changed mission realization")
	}
	if first := controls[1].FirstAuto; first != 0 {
		var before, after *radarTick
		for i := range controls[0].Ticks {
			if controls[0].Ticks[i].Step == first {
				before = &controls[0].Ticks[i]
			}
		}
		for i := range controls[1].Ticks {
			if controls[1].Ticks[i].Step == first {
				after = &controls[1].Ticks[i]
			}
		}
		if before == nil || after == nil || before.RNG != after.RNG || before.Pose != after.Pose {
			t.Fatal("pre-automatic paired state/RNG diverged", first, before, after)
		}
		t.Logf("paired first auto step%d phase%d RNG%04X NoneAEB%04X CMAEB%04X chaff%d→%d", first, after.Phase, after.RNG, before.Threat, after.Threat, before.Defenses.Chaff, after.Defenses.Chaff)
	}
}
