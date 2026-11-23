package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"testing"
	"time"
)

// Camera endpoints are deliberately supplied so all ownship view owners can be
// exercised without making a parked aircraft move to manufacture a pan. The
// actual preflight/carrier mast integration is covered independently below.
func TestPlayerContactDoesNotCollapseMovingView(t *testing.T) {
	for _, mode := range []uint8{0, 3, 7} {
		for _, presentation := range []string{"smooth", "stepped"} {
			t.Run(fmt.Sprintf("view%d-%s", mode, presentation), func(t *testing.T) {
				p := &RenderingPreview{Initialized: &gamestate.InitializedMission{PlayerID: "player"}, ViewControls: &render3d.CameraController{Mode: mode}, Renderer: &render3d.Renderer{Assets: &world.Bundle{Sine: make([]int16, 1025)}}}
				p.presentation.mode = presentation
				p.resetPresentation()
				seen := map[uint16]bool{}
				for tick := 0; tick < 10; tick++ {
					p.beginPresentationStep()
					p.Scene.Camera.Angles[1] += 600
					p.endPresentationStep()
					if p.presentationDiscontinuity(false, true, false) {
						p.resetPresentation()
					}
					for frame := 0; frame < 6; frame++ {
						p.presentation.alpha = float64(frame) / 6
						scene, _ := p.presentationInputs()
						seen[scene.Camera.Angles[1]] = true
					}
				}
				want := 10
				if presentation == "smooth" {
					want = 60
				}
				if len(seen) != want || p.Scene.Camera.Angles[1] != 6000 {
					t.Fatalf("contact transitions collapsed display or changed endpoint: %d/%d, yaw%d", len(seen), want, p.Scene.Camera.Angles[1])
				}
				// A real positional jump remains a discontinuity. This does not
				// change stepped semantics or the authoritative camera endpoint.
				p.Scene.Camera.H0 = 0x70000
				if !p.presentationDiscontinuity(false, true, true) {
					t.Fatal("world-boundary snap lost")
				}
				p.resetPresentation()
				if p.presentation.previous.Camera != p.Scene.Camera || p.presentation.current.Camera != p.Scene.Camera {
					t.Fatal("boundary reset retained stale position")
				}
			})
		}
	}
}

func TestActualStationaryMastPanContactCadence(t *testing.T) {
	for _, family := range []string{"carrier", "land"} {
		for _, presentation := range []string{"smooth", "stepped"} {
			t.Run(family+"-"+presentation, func(t *testing.T) {
				c := mastFixture(t, 1, family)
				jFinishFade(t, c)
				p, r := c.Rendering, c.Rendering.Flight
				act(t, c, Actions{ViewKey: 2})
				act(t, c, Actions{})
				p.presentation.mode = presentation
				p.resetPresentation()
				pose := r.State.Pose
				step := r.Steps
				contact, transitions := r.State.ValidContact, 0
				seen := map[render3d.Camera]bool{}
				for frame := 1; frame <= 60; frame++ {
					act(t, c, Actions{InputTime: time.Duration(frame) * time.Second / 60, FlightHeldValid: true, SceneDirections: [4]bool{true}})
					if err := c.AdvanceRendering(1.0 / 60); err != nil {
						t.Fatal(err)
					}
					if c.Screen != ScreenScene || r.Failure != "" {
						t.Fatalf("source fixture stopped: %s %s", c.Screen, r.Failure)
					}
					scene, _ := p.presentationInputs()
					seen[scene.Camera] = true
					if contact != r.State.ValidContact {
						transitions++
						contact = r.State.ValidContact
					}
				}
				if transitions == 0 || r.Steps-step < 9 || r.Steps-step > 11 {
					t.Fatalf("contact/clock control not exercised: %d transitions/%d ticks", transitions, r.Steps-step)
				}
				if presentation == "smooth" && len(seen) < 45 || presentation == "stepped" && len(seen) > 12 {
					t.Fatalf("wrong display cadence: %s %d/60 distinct cameras", presentation, len(seen))
				}
				if pose.H0 != r.State.Pose.H0 || pose.H1 != r.State.Pose.H1 || pose.Angles != r.State.Pose.Angles {
					t.Fatal("mast pan moved airframe", pose, r.State.Pose)
				}
				before, _ := json.Marshal(p.FilmCheckpoint())
				for i := 0; i < 3; i++ {
					p.Dirty = true
					c.Frame(false)
				}
				after, _ := json.Marshal(p.FilmCheckpoint())
				if p.Error != nil || !bytes.Equal(before, after) {
					t.Fatal("extra intermediate drawing changed authoritative checkpoint", p.Error)
				}
				t.Logf("native %s %s: %d logical ticks, %d contact transitions, %d distinct cameras/60 samples; stationary H0/H1/angles", family, presentation, r.Steps-step, transitions, len(seen))
			})
		}
	}
}
