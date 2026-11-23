package menus

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gunship2000-assets/game/flight"
)

func TestPlayerControlAndDynamicParticipationStayDistinct(t *testing.T) {
	for aircraft := 0; aircraft < 8; aircraft++ {
		for _, start := range []string{"land", "carrier", "pass"} {
			t.Run(fmt.Sprintf("air%d-%s", aircraft, start), func(t *testing.T) {
				c := mastFixture(t, aircraft, start)
				jFinishFade(t, c)
				p := c.Rendering
				e := p.Initialized.Entities[p.Initialized.PlayerRow]
				// Original zero row2582+1; loader08f1 writes Control0488+1.
				if e.Kind != 0xff || e.Participation != 0 || e.DynamicRaw[1] != 0 || e.ControlRaw[1] != 1 || !e.Visible {
					t.Fatalf("player/control domain mismatch: kind%x P%d dynamic%x control%x visible%v", e.Kind, e.Participation, e.DynamicRaw[1], e.ControlRaw[1], e.Visible)
				}
				if err := c.Handle(Actions{ViewKey: 5}); err != nil {
					t.Fatal(err)
				}
				// Physical F5 selects camera mode3; F3 instead selects internal mode1.
				p.filterSceneInstances()
				ownship := false
				for _, v := range p.Scene.Instances {
					ownship = ownship || v.ID == p.Initialized.PlayerID
				}
				if !ownship {
					t.Fatal("source P0 removed separately registered exterior ownship")
				}
			})
		}
	}
}

func TestTapAndSpeechLogicalStateIndependentOfDrawing(t *testing.T) {
	var expected []byte
	for _, mode := range []string{"smooth", "stepped"} {
		for _, scale := range []int{1, 2} {
			for _, fps := range []int{30, 60, 144} {
				t.Run(fmt.Sprintf("%s-%d-%d", mode, scale, fps), func(t *testing.T) {
					c := armedMission(t)
					p, r := c.Rendering, c.Rendering.Flight
					e := p.Initialized.Entities[24] // Unchanged naturally generated towed gun.
					r.State.Pose = flight.Pose{H0: e.Pose.H0, H1: e.Pose.H1 - 3000, Height: int16(e.Pose.Height + 256)}
					r.State.Forward, r.State.Rotor, r.State.Collective, r.State.Hover, r.State.HeightControl, r.State.HeightTarget, r.State.CruiseSpeed = 0, 1, 16, 1, 1, int16(e.Pose.Height+256), 0
					r.Avionics.Target = 25
					p.publishFlight()
					if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
						t.Fatal(err)
					}
					p.publishView()
					p.Scene.RenderScale, p.presentation.mode = scale, mode
					p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
					p.resetPresentation()
					c.Handle(Actions{WeaponTriggerValid: true})
					for _, at := range []time.Duration{10, 510} {
						r.commandInput.SampleAt(at*time.Millisecond, Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
						r.commandInput.SampleAt((at+15)*time.Millisecond, Actions{WeaponTriggerValid: true})
					}
					last := time.Duration(0)
					for n := 1; n <= fps*2; n++ {
						now := time.Duration(n) * time.Second / time.Duration(fps)
						if err := c.AdvanceRendering(float64(now-last) / float64(time.Second)); err != nil {
							t.Fatal(err)
						}
						last = now
						p.Dirty = true
						c.Frame(false)
						if p.Error != nil {
							t.Fatal(p.Error)
						}
					}
					audio := [][2]uint16{}
					voices := 0
					for _, event := range r.sceneAudio {
						audio = append(audio, [2]uint16{event.event, event.argument})
						if event.event == 0x2d || event.event == 0x2e || event.event == 0x30 {
							voices++
						}
					}
					attempts := 0
					for _, event := range r.TriggerTrace {
						if event.Phase == "attempt" && event.Reason == "trigger boundary" {
							attempts++
						}
					}
					if voices == 0 || attempts != 2 {
						t.Fatalf("fixture missed speech/taps: voices%d attempts%d", voices, attempts)
					}
					state, err := json.Marshal([]any{p.Initialized, r.State, r.Avionics, r.AcquisitionSpeech, p.combatRaw(), r.CombatEvents, audio})
					if err != nil {
						t.Fatal(err)
					}
					if expected == nil {
						expected = state
					} else if !reflect.DeepEqual(expected, state) {
						t.Fatal("drawing changed logical flight/contact/RNG/combat/speech control state")
					}
				})
			}
		}
	}
}

func TestAltCSpeechGateUsesNativeEdgeWithoutStoppingVoices(t *testing.T) {
	c := armedMission(t)
	r := c.Rendering.Flight
	if !r.SpeechEnabled {
		t.Fatal("selected complete driver must initially admit source DE80")
	}
	var latch inputLatch
	// This uses the same Buttons-to-Actions edge owner as the native window.
	if err := c.Handle(latch.Read(Buttons{AltC: true})); err != nil {
		t.Fatal(err)
	}
	if r.SpeechEnabled || c.Rendering.combatInput(6, 0, nil).SpeechEnabled {
		t.Fatal("Alt+C did not disable future speech producers")
	}
	n := len(r.sceneAudio)
	c.Handle(latch.Read(Buttons{AltC: true, InputTime: time.Millisecond}))
	if r.SpeechEnabled || len(r.sceneAudio) != n {
		t.Fatal("held Alt+C retriggered or stopped/rewrote active audio commands")
	}
	c.Handle(latch.Read(Buttons{InputTime: 2 * time.Millisecond}))
	c.Handle(latch.Read(Buttons{AltC: true, InputTime: 3 * time.Millisecond}))
	if !r.SpeechEnabled || len(r.sceneAudio) != n {
		t.Fatal("second physical edge failed or manufactured an audio event")
	}
}

func armedLongbowMission(t *testing.T) *FrontEnd {
	t.Helper()
	c := mastFixture(t, 1, "pass")
	jFinishFade(t, c)
	if err := c.returnRenderingBriefing(); err != nil {
		t.Fatal(err)
	}
	if err := c.openOrdnance(0); err != nil {
		t.Fatal(err)
	}
	choose := func(action string, system, station int) {
		t.Helper()
		for i, v := range c.Mission.Ordnance.Choices {
			if v.Enabled && v.Action == action && (system < 0 || v.System == system) && (station < 0 || v.Station == station) {
				missionClick(t, c, i)
				return
			}
		}
		t.Fatalf("missing accepted choice %s/%d/%d", action, system, station)
	}
	for _, v := range []struct{ system, station int }{{7, 0}, {5, 1}} {
		choose("fits", v.system, -1)
		choose("mount", v.system, v.station)
		choose("back", -1, -1)
	}
	choose("back", -1, -1)
	if c.Screen == ScreenOutfit {
		missionClick(t, c, 6)
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("loadout acceptance", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	return c
}
