package menus

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gunship2000-assets/game/flight"
)

// Source1:a9b installs target mode10 without0bh. The mode table1f8e visits
// projectile expiry1a68 only for mode6, so changing toF8 deliberately leaves
// ASOUND17's retained row6 until a later real0bh caller (for exampleF1).
// This is a source compatibility test, not permission to stop every sound when
// a target dies. Pose is supplied; shot, collision and destruction are real.
func TestHellfireViewSoundSourceLifetime(t *testing.T) {
	for _, tc := range []struct{ leave, suppliedDesignation bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		leave := tc.leave
		name := "remain-F9"
		if leave {
			name = "F9-F8"
		}
		if tc.suppliedDesignation {
			name += "-supplied-designation-hit"
		} else {
			name += "-commanded-designation-loss-miss"
		}
		t.Run(name, func(t *testing.T) {
			c := armedMission(t)
			p, r := c.Rendering, c.Rendering.Flight
			row := -1
			for i, e := range p.Initialized.Entities {
				if i > 4 && e.Participation == 1 && e.Kind == 4 && e.Object.Raw[0]&0x10 != 0 && int16(binary.LittleEndian.Uint16(e.Object.Raw[29:])) > 0 {
					row = i
					break
				}
			}
			if row < 0 {
				t.Fatal("generated stationary target")
			}
			e := p.Initialized.Entities[row]
			r.State.Pose = flight.Pose{H0: e.Pose.H0, H1: e.Pose.H1 - 3000, Height: int16(e.Pose.Height + 256)}
			p.publishFlight()
			p.Initialized.SelectedStation = 2 // accepted Hellfire, not a test projectile
			r.Avionics.Target, p.Cockpit.TargetSymbol = uint16(row+1), 1
			c.Handle(Actions{WeaponTriggerValid: true})
			in := p.combatInput(6, 1, func() uint16 { return 1 })
			if err := r.Combat.Fire(in); err != nil {
				t.Fatal(err)
			}
			if r.Combat.Slots[r.Combat.NextSlot].Lifetime == 0 {
				t.Fatal("real launch failed")
			}
			p.collectCombatEvents()
			p.takeSceneAudio()
			if err := c.Handle(Actions{ViewKey: 9}); err != nil {
				t.Fatal(err)
			}
			if p.ViewControls.Mode != 6 {
				t.Fatal("F9 not entered")
			}
			q := p.takeSceneAudio()
			if len(q) != 1 || q[0].event != 0x17 {
				t.Fatal("source17 entry", q)
			}
			if leave {
				if err := c.Handle(Actions{ViewKey: 8}); err != nil {
					t.Fatal(err)
				}
				if p.ViewControls.Mode != 10 {
					t.Fatal("F8 not entered")
				}
				if q := p.takeSceneAudio(); len(q) != 0 {
					t.Fatal("invented camera audio", q)
				}
			}
			if !tc.suppliedDesignation {
				// The current source-None seed10 has a clear ray where the old
				// seed1 scene lost designation naturally. Exercise loss through the
				// real target/standby consumer, without editing projectile state
				// or pretending the old terrain/Support predecessor is unchanged.
				if err := c.Handle(Actions{Backspace: true}); err != nil {
					t.Fatal(err)
				}
			}
			type point struct {
				Visit               int
				Step                uint64
				Mode                uint8
				Life, Tail          byte
				Kind, Participation uint8
				Events              []uint16
			}
			var timeline []point
			initialMode := p.ViewControls.Mode
			hit, retired, restored := false, false, false
			for n := 0; n < 150; n++ {
				if tc.suppliedDesignation {
					// Explicit accepted designation/collision input, as in the
					// retained M stationary-hit fixture. This isolates the
					// sound successor without claiming natural lock retention.
					in.Phase = uint16(n + 1)
					if err := p.advanceCombat(in); err != nil {
						t.Fatal(err)
					}
				} else if err := c.AdvanceRendering(.1); err != nil {
					t.Fatal(err)
				}
				slot := r.Combat.Slots[r.Combat.NextSlot]
				v := p.Initialized.Entities[row]
				pt := point{Visit: n + 1, Step: r.Steps, Mode: p.ViewControls.Mode, Life: slot.Lifetime, Tail: slot.Packed, Kind: v.Kind, Participation: v.Participation}
				for _, q := range p.takeSceneAudio() {
					pt.Events = append(pt.Events, q.event)
					if q.event == 0xb {
						restored = true
					}
				}
				timeline = append(timeline, pt)
				if v.Participation == 2 || v.Kind == 0x1c || v.Kind == 0x1d {
					hit = true
				}
				if slot.Lifetime == 0 && slot.Packed == 0 {
					retired = true
					break
				}
			}
			if hit != tc.suppliedDesignation || !retired {
				t.Fatal("must reach real target destruction and complete projectile tail", hit, retired, timeline)
			}
			if tc.suppliedDesignation {
				// The next ordinary reached camera phase owns the0bh return.
				if err := c.AdvanceRendering(.1); err != nil {
					t.Fatal(err)
				}
				for _, q := range p.takeSceneAudio() {
					if q.event == 0xb {
						restored = true
					}
				}
			}
			if leave {
				if p.ViewControls.Mode == 6 || restored {
					t.Fatal("target view improperly ran projectile-only sound successor", timeline)
				}
				if err := c.Handle(Actions{ViewKey: 1}); err != nil {
					t.Fatal(err)
				}
				q := p.takeSceneAudio()
				if len(q) != 1 || q[0].event != 0xb {
					t.Fatal("F1 must restore row6", q)
				}
			} else if p.ViewControls.Mode == initialMode || !restored {
				t.Fatal("F9 expiry must issue0b", timeline)
			}
			if root := os.Getenv("GS2000_O2_AUDIO_CAPTURE"); root != "" {
				if err := os.MkdirAll(root, 0755); err != nil {
					t.Fatal(err)
				}
				b, _ := json.MarshalIndent(timeline, "", "  ")
				if err := os.WriteFile(filepath.Join(root, name+".json"), b, 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
