package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

// These cases use genuine prepared land/carrier sessions and the production
// controller update. Poses/readiness are explicit diagnostic boundary inputs;
// they are not reproductions of the user's unrecorded landing/prompt state.
func TestNaturalBasePromptNegativeControllerPaths(t *testing.T) {
	for _, start := range []string{"land", "carrier"} {
		for _, scenario := range []string{"airborne-stale-contact", "rotor-running", "cooldown", "accepted"} {
			t.Run(fmt.Sprintf("%s/%s", start, scenario), func(t *testing.T) {
				c := mastFixture(t, 0, start)
				jFinishFade(t, c)
				p, session := c.Rendering, c.Mission
				r := p.Flight
				saved := c.Store.State()
				r.State.FlightStage = 0
				r.State.Rotor = 0
				switch scenario {
				case "airborne-stale-contact":
					r.State.Pose.Height += 128
					r.State.RadarAltitude = 128
					r.State.ValidContact = true
				case "rotor-running":
					r.State.Rotor = 1
				case "cooldown":
					r.State.FlightStage = 128
				}
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
				pending := p.Initialized.Sortie != nil && p.Initialized.Sortie.Pending != ""
				if scenario == "accepted" {
					if !pending || c.Screen != ScreenQuit || p.Initialized.Sortie.Pending != "base" || !r.State.ValidContact {
						t.Fatalf("accepted base missing prompt: screen%s contact%v sortie%+v", c.Screen, r.State.ValidContact, p.Initialized.Sortie)
					}
				} else if pending || c.Screen != ScreenScene {
					t.Fatalf("premature natural prompt: screen%s state%+v sortie%+v", c.Screen, r.State, p.Initialized.Sortie)
				}
				if scenario == "airborne-stale-contact" && r.State.ValidContact {
					t.Fatal("new airborne update reused previous accepted contact")
				}
				if c.Mission != session || !reflect.DeepEqual(c.Store.State(), saved) {
					t.Fatal("contact/readiness check changed persistent or mission ownership")
				}
			})
		}
	}
}

func TestLandingPreferencesThroughAcceptedLaunchAndContact(t *testing.T) {
	for _, start := range []string{"land", "carrier"} {
		for _, landings := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/landings%d", start, landings), func(t *testing.T) {
				c := mastFixture(t, 0, start)
				jFinishFade(t, c)
				act(t, c, Actions{EndMission: true})
				act(t, c, Actions{ConfirmQuit: true})
				waitForResultVerification(t, c)
				c.Mission.Settings.Landings = landings
				startRendering(t, c)
				jFinishFade(t, c)
				p := c.Rendering
				flags := p.Initialized.Switches
				if (flags&0x20 != 0) != (landings == 0) || (flags&0x400 != 0) != (c.Mission.Data.Inputs.MissionType == gamestate.MissionTraining) {
					t.Fatalf("UI/source flag domains collapsed: settings%d type%v flags%04x", landings, c.Mission.Data.Inputs.MissionType, flags)
				}
				s := p.Flight.State
				s.Pose.Height = s.ContactHeight + 3
				s.Vertical, s.Rotor = -20, 1
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
				if landings == 0 {
					if s.Terminal != nil || !s.ValidContact {
						t.Fatalf("source flat NoCrashes contact not protected: terminal%+v contact%v", s.Terminal, s.ValidContact)
					}
				} else if s.Terminal == nil || s.ValidContact {
					t.Fatalf("realistic unsafe contact failed to enter terminal: flags%04x terminal%+v", flags, s.Terminal)
				}
			})
		}
	}
}

func TestPassCorridorIsIndependentOfOrdinaryBaseContact(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p, session := c.Rendering, c.Mission
	base := session.Data.Objectives["base"].Point
	s := p.Flight.State
	s.FlightStage, s.Rotor, s.Pose.Height = 0, 1, 512
	s.Pose.H0, s.Pose.H1 = uint32(base.X), uint32(base.Y)
	// Choose the actual retained start's nearest boundary. The expected source
	// threshold is4096 shifted units; no fabricated opposite exit is introduced.
	distance, axis, edge := int32(1<<30), 0, int32(0)
	for _, arm := range []struct {
		axis              int
		position, shifted int32
	}{{0, base.X, 0x400}, {0, base.X, 0x7bff}, {1, base.Y, 0x400}, {1, base.Y, 0x7bff}} {
		d := (arm.position >> 4) - arm.shifted
		if d < 0 {
			d = -d
		}
		if d < distance {
			distance, axis, edge = d, arm.axis, arm.shifted
		}
	}
	if distance >= 4096 {
		t.Fatalf("fixture has no source corridor beside its actual start: %+v", base)
	}
	outside := uint32(0x3f00)
	if edge == 0x7bff {
		outside = 0x7c100
	}
	if axis == 0 {
		s.Pose.H0 = outside
	} else {
		s.Pose.H1 = outside
	}
	if err := c.advanceFlight(.1); err != nil {
		t.Fatal(err)
	}
	if s.ValidContact || s.Rotor != 1 || p.Initialized.Sortie.Pending != "corridor" || c.Screen != ScreenQuit || c.Mission != session {
		t.Fatalf("corridor incorrectly acquired ordinary base guards: contact%v rotor%d screen%s sortie%+v", s.ValidContact, s.Rotor, c.Screen, p.Initialized.Sortie)
	}
}
