package menus

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gunship2000-assets/game/flight"
)

func instanceModels(p *RenderingPreview, id string) []int {
	var out []int
	for _, in := range p.Scene.Instances {
		if in.ID == id {
			out = append(out, in.ModelID)
		}
	}
	return out
}

func TestReservePickupRegistrationUsesRealLifecycle(t *testing.T) {
	var evidence []any
	for _, actor := range []int{1, 4} {
		for _, scale := range []int{1, 2} {
			for _, mode := range []string{"smooth", "stepped"} {
				t.Run(fmt.Sprintf("actor%d/scale%d/%s", actor, scale, mode), func(t *testing.T) {
					c := developmentScenarioController(t, "n6-rescue")
					p := c.Rendering
					m := p.Initialized
					r := p.Flight
					p.Scene.RenderScale = scale
					p.presentation.mode = mode
					// Supplied approach height64 allows the real reserve to remain collectible
					// for several iterations. The4→8 case redirects the failed actor,
					// then supplies the revived actor1 above pickup height: P4's real
					// ascending carrier phase otherwise lets that healthy Blackhawk
					// collect reserve8 immediately at the shared low/slow pose.
					r.State.Pose.Height = 64
					r.State.HeightTarget = 64
					if actor == 4 {
						e := &m.Entities[4]
						original := m.Entities[1]
						e.Kind, e.Participation, e.Pose, e.RawMotion = 0x20, 1, original.Pose, 0
						e.ControlRaw[1], e.ControlRaw[5] = 1, 0
						binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
						e.DynamicRaw[0], e.DynamicRaw[1] = 0x20, 1
						binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
						binary.LittleEndian.PutUint32(e.DynamicRaw[14:], e.Pose.H0)
						binary.LittleEndian.PutUint32(e.DynamicRaw[18:], e.Pose.H1)
						binary.LittleEndian.PutUint16(e.DynamicRaw[22:], e.Pose.Height)
						r.Systems.Fuel[1] = 100
						r.Systems.Fuel[4] = 0
						m.Stores.Ships[1].Fuel = 100
						m.Stores.Ships[4].Fuel = 0
						m.Entities[1].Pose.Height = 256
						binary.LittleEndian.PutUint16(m.Entities[1].DynamicRaw[22:], 256)
					}
					p.publishFlight()
					p.resetPresentation()
					reserve := actor + 4
					if m.Entities[reserve].Participation != 0 || len(instanceModels(p, m.Entities[reserve].ID)) != 0 {
						t.Fatal("inactive reserve shown before real failure")
					}
					cargo, mass := m.Stores.Ships[0].Flags, r.MassTerm
					seenActive, seenP0, seenP1, collected := false, false, false, false
					if err := c.Handle(Actions{Cancel: true}); err != nil {
						t.Fatal(err)
					}
					for n := 0; n < 80; n++ {
						if err := c.AdvanceRendering(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
							t.Fatal(err)
						}
						e, s := m.Entities[actor], m.Entities[reserve]
						shown := instanceModels(p, s.ID)
						if e.Kind == 0x20 && e.Participation == 0 {
							seenP0 = true
							if len(instanceModels(p, e.ID)) != 0 {
								t.Fatal("ordinary P0 is not source-registerable")
							}
						}
						if seenP0 && e.Kind == 0x20 && e.Participation == 1 {
							seenP1 = true
							if len(instanceModels(p, e.ID)) != 1 {
								t.Fatal("later source P1 helicopter did not return")
							}
						}
						if s.Kind == 0x1b && s.Participation == 1 {
							seenActive = true
							if !reflect.DeepEqual(shown, []int{s.ModelID}) {
								t.Fatal("separate source Pickup missing/duplicate/wrong model", shown, s.ModelID)
							}
							if s.ModelID != 151 && s.ModelID != 227 {
								t.Fatal("fixture did not select an established source Pickup", s.ModelID)
							}
							for _, in := range p.Scene.Instances {
								if in.ID == s.ID {
									if in.Pose.H0 != s.Pose.H0 || in.Pose.H1 != s.Pose.H1 || in.Pose.Height != s.Pose.Height || in.Angles != s.Pose.Angles || in.Callback != uint8(binary.LittleEndian.Uint16(s.Object.Raw[:])>>8) || in.Oriented != (binary.LittleEndian.Uint16(s.Object.Raw[:])&8 != 0) {
										t.Fatal("source registration metadata/pose mismatch")
									}
								}
							}
						}
						if s.Kind == 0x41 {
							t.Logf("reserve%d collected by actual actor%d at step%d; failed actor%d kind%02x/P%d; playerH%d; carrierH%d", reserve, s.DynamicRaw[3], r.Steps, actor, e.Kind, e.Participation, r.State.Pose.Height, m.Entities[int(s.DynamicRaw[3])].Pose.Height)
							collected = true
							if s.DynamicRaw[3] != 0 {
								t.Fatal("player rescue fixture was collected by another actual carrier", s.DynamicRaw[3])
							}
							if len(shown) != 0 {
								t.Fatal("collected41 visible")
							}
						}
						before, _ := json.Marshal(struct {
							Entities any
							RNG      uint16
							Fuel     []int16
							Mass     int16
						}{m.Entities, m.RandomState, r.Systems.Fuel, r.MassTerm})
						p.filterSceneInstances()
						p.filterSceneInstances()
						after, _ := json.Marshal(struct {
							Entities any
							RNG      uint16
							Fuel     []int16
							Mass     int16
						}{m.Entities, m.RandomState, r.Systems.Fuel, r.MassTerm})
						if !reflect.DeepEqual(before, after) {
							t.Fatal("registration mutated authoritative state")
						}
						evidence = append(evidence, map[string]any{"actor": actor, "scale": scale, "mode": mode, "step": r.Steps, "kind": s.Kind, "P": s.Participation, "model": s.ModelID, "shown": shown})
						if collected {
							break
						}
					}
					if !seenActive || !seenP0 || !seenP1 || !collected {
						t.Fatal("real lifecycle coverage", seenActive, seenP0, seenP1, collected)
					}
					if m.Stores.Ships[0].Flags != cargo || r.MassTerm != mass {
						t.Fatal("survivor rendering/pickup invented cargo or mass")
					}
				})
			}
		}
	}
	if output := os.Getenv("GS2000_O_PRESENTATION_EVIDENCE"); output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.MarshalIndent(evidence, "", "  ")
		if err := os.WriteFile(filepath.Join(output, "reserve-lifecycle.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAirborneFailureDoesNotCreatePickup(t *testing.T) {
	c := developmentScenarioController(t, "n6-follower-empty")
	p := c.Rendering
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 250 && p.Initialized.Entities[1].Kind != 0x1c; n++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if p.Initialized.Entities[1].Kind != 0x1c {
		t.Fatal("real P2 fall/wreck not reached")
	}
	for row := 5; row < 9; row++ {
		e := p.Initialized.Entities[row]
		if e.Participation != 0 || len(instanceModels(p, e.ID)) != 0 {
			t.Fatal("airborne failure invented or displayed reserve", row)
		}
	}
}
