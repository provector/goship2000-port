package menus

import (
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/flight"
	"testing"
)

func TestRealTerminalRequiresDeliveryWithoutDrawMutation(t *testing.T) {
	for _, scale := range []int{1, 2} {
		for _, mode := range []string{"smooth", "stepped"} {
			t.Run(fmt.Sprintf("%d/%s", scale, mode), func(t *testing.T) {
				c := developmentScenarioController(t, "n6-critical-fuel")
				p, r := c.Rendering, c.Rendering.Flight
				p.Scene.RenderScale = scale
				p.presentation.mode = mode
				p.resetPresentation()
				r.RequireTerminalAck = true
				if err := c.Handle(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				r.Compression.Request = 16
				if err := c.AdvanceRendering(.25); err != nil {
					t.Fatal(err)
				}
				if r.State.Terminal == nil || r.TerminalPresentation.Frames != 1 || !r.terminalAwaitingAck || r.Compression.Request != 0 {
					t.Fatal("entry not published", r.State.Terminal, r.TerminalPresentation)
				}
				for !r.TerminalPresentation.Completed {
					beforeRNG := p.Initialized.RandomState
					beforeCount := r.State.Terminal.Countdown
					if err := c.AdvanceRendering(.25); err != nil {
						t.Fatal(err)
					}
					if r.State.Terminal.Countdown != beforeCount || p.Initialized.RandomState != beforeRNG {
						t.Fatal("waiting delivery progressed logical effects")
					}
					c.Frame(false)
					c.Frame(false)
					if p.Error != nil || p.Initialized.RandomState != beforeRNG {
						t.Fatal("redraw mutated source", p.Error)
					}
					c.MarkScenePresented()
					if err := c.AdvanceRendering(.25); err != nil {
						t.Fatal(err)
					}
				}
				if r.TerminalPresentation.Frames != 4 || r.TerminalPresentation.Fade || c.Screen != ScreenDebrief {
					t.Fatal("final jump/delivery", r.TerminalPresentation, c.Screen)
				}
			})
		}
	}
}

func TestCompressionNormalLogicalStateEquivalence(t *testing.T) {
	// Real generated objective, flight, projectiles, systems and shared RNG. The
	// comparison stops after the same number of ordinary logical steps, not seconds.
	var expected []byte
	for _, rate := range []int16{0, 2, 4, 16} {
		for _, fps := range []int{30, 60, 144} {
			c := developmentScenarioController(t, "n6-compression")
			p, r := c.Rendering, c.Rendering.Flight
			c.Handle(Actions{Cancel: true})
			c.Handle(Actions{})
			r.Compression.Request = rate
			for r.Steps < 20 {
				dt := 1.0 / float64(fps)
				if 20-r.Steps < 8 {
					dt = float64(flight.StepDuration()) / 1e9 / float64(r.Compression.Rate()) / 2
				}
				if err := c.AdvanceRendering(dt); err != nil {
					t.Fatal(err)
				}
			}
			if r.Steps != 20 {
				t.Fatal("incorrect reached step count", rate, fps, r.Steps)
			}
			snapshot, err := json.Marshal(struct {
				State   *flight.State
				Systems flight.Systems
				Targets any
				Slots   any
				RNG     uint16
				Phase   uint16
			}{r.State, r.Systems, p.ResultTargets, r.Combat.Slots, p.Initialized.RandomState, r.ElapsedDA})
			if err != nil {
				t.Fatal(err)
			}
			if expected == nil {
				expected = snapshot
			} else if string(expected) != string(snapshot) {
				t.Fatalf("logical mismatch rate%d fps%d", rate, fps)
			}
			before := p.Initialized.RandomState
			c.Frame(false)
			c.Frame(false)
			if p.Initialized.RandomState != before {
				t.Fatal("draw RNG")
			}
		}
	}
}
func TestBracketLatchAndModalReset(t *testing.T) {
	var l inputLatch
	a := l.Read(Buttons{CompressionIncrease: true})
	b := l.Read(Buttons{CompressionIncrease: true})
	if !a.CompressionIncrease || b.CompressionIncrease {
		t.Fatal("held bracket duplicated edge")
	}
	c := developmentScenarioController(t, "n6-compression")
	c.Handle(Actions{Cancel: true})
	c.Handle(Actions{})
	c.Handle(Actions{CompressionIncrease: true})
	c.Handle(Actions{CompressionIncrease: true})
	if c.Rendering.Flight.Compression.Request != 4 {
		t.Fatal("brackets not wired")
	}
	c.Handle(Actions{FlightPause: true})
	if c.Rendering.Flight.Compression.Request != 0 {
		t.Fatal("pause compression")
	}
}
