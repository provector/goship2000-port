package menus

import (
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/combat"
	"gunship2000-assets/game/flight"
	"testing"
)

func TestIncomingRealMechanicsIndependentOfPresentationAndRate(t *testing.T) {
	var expected []byte
	for _, mode := range []string{"smooth", "stepped"} {
		for _, scale := range []int{1, 2} {
			for _, rate := range []int16{0, 4} {
				t.Run(fmt.Sprintf("%s/%d/%d", mode, scale, rate), func(t *testing.T) {
					c := developmentScenarioController(t, "n6-incoming")
					p, r := c.Rendering, c.Rendering.Flight
					p.Scene.RenderScale = scale
					p.presentation.mode = mode
					p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
					p.resetPresentation()
					if !p.Paused || r.Combat.Slots[20].Lifetime != 30 || r.State.Terminal != nil {
						t.Fatal("fixture not paused real-mechanics boundary")
					}
					if err := c.Handle(Actions{Cancel: true}); err != nil {
						t.Fatal(err)
					}
					r.Compression.Request = rate
					for n := 0; n < 8000 && r.Combat.Slots[20].Lifetime != 0 && c.Screen == ScreenScene; n++ {
						updates := r.Combat.Updates
						// Less than one quantum even at4x: compare the exact same stopping
						// logical step, rather than compare overshot batches after contact.
						if err := c.AdvanceRendering(.001); err != nil {
							t.Fatal(err)
						}
						if updates != r.Combat.Updates {
							c.Frame(false)
							c.MarkScenePresented()
							if p.Error != nil {
								t.Fatal(p.Error)
							}
						}
					}
					if r.Combat.Slots[20].Lifetime != 0 || len(r.DamageEvents) == 0 {
						t.Fatal("real contact/7468 not reached", r.Combat.Slots[20])
					}
					beforeRNG := p.Initialized.RandomState
					c.Frame(false)
					c.Frame(false)
					if p.Initialized.RandomState != beforeRNG {
						t.Fatal("draw consumed shared RNG")
					}
					raw, err := json.Marshal(struct {
						Flight  *flight.State
						Systems flight.Systems
						Slots   [combat.SlotCount]combat.Projectile
						RNG     uint16
						Updates uint64
						Damage  []PlayerHitWitness
						Results [4]combat.Result
					}{r.State, r.Systems, r.Combat.Slots, p.Initialized.RandomState, r.Combat.Updates, r.DamageEvents, r.Combat.Results})
					if err != nil {
						t.Fatal(err)
					}
					if expected == nil {
						expected = raw
					} else if string(raw) != string(expected) {
						t.Fatal("same logical incoming input changed authoritative state/RNG with presentation/rate")
					}
				})
			}
		}
	}
}
