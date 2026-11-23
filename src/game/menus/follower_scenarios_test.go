package menus

import (
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestPreparedFollowerScenariosReachRealSuccessors(t *testing.T) {
	for _, name := range []string{"n6-rescue", "n6-follower-empty"} {
		t.Run(name, func(t *testing.T) {
			c := developmentScenarioController(t, name)
			p, r := c.Rendering, c.Rendering.Flight
			m := p.Initialized
			if m.Entities[5].Participation != 0 || m.Entities[5].Kind != 0x1b || r.Systems.Fuel[1] != 0 || r.State.Terminal != nil {
				t.Fatal("fixture prefilled a rescue or terminal")
			}
			if err := c.Handle(Actions{Cancel: true}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 250 && m.Entities[5].Kind != 0x41 && (name == "n6-rescue" || m.Entities[1].Kind != 0x1c); i++ {
				if err := c.AdvanceRendering(.12); err != nil {
					t.Fatal(err)
				}
			}
			if r.Systems.UnsupportedFollowerEmptyBurn != 0 || !m.FollowerFailureKnown[1] {
				t.Fatal("source chain provenance missing", r.Systems.UnsupportedFollowerEmptyBurn, m.FollowerLimits)
			}

			if name == "n6-rescue" {
				for i := 0; i < 10 && m.Entities[5].Kind != 0x41; i++ {
					if err := c.AdvanceRendering(.12); err != nil {
						t.Fatal(err)
					}
				}
				if m.Entities[5].Kind != 0x41 || m.Entities[5].DynamicRaw[3] != 0 {
					t.Fatal("real source-positioned pickup missing")
				}
			} else if m.Entities[1].Kind != 0x1c || m.Stores.Damage[1][9] != 1 || m.Entities[5].Participation != 0 {
				t.Fatal("source skill127 aloft branch did not produce real P2 fall/wreck without invented rescue")
			}
		})
	}
}

func TestActualReservePickupReturnFreezeCommitRestart(t *testing.T) {
	c := developmentScenarioController(t, "n6-rescue")
	p := c.Rendering
	m := p.Initialized
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40 && m.Entities[5].Kind != 0x41; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if m.Entities[5].Kind != 0x41 {
		t.Fatal("real pickup absent")
	}
	// Labelled staged approach preserves collection; ordinary contact/rotor prompt
	// creates the actual return flag. It is not a continuously flown return claim.
	if err := c.developmentScenarioPose(c.DevelopmentScenario.BaseTARGET, true, 2, 1, 0); err != nil {
		t.Fatal(err)
	}
	scenarioSafeReturn(t, c)
	proposal := c.Debrief.Pending.Proposal
	context := proposal.Facts.ResultContext
	wantVersion, frozenScene := 2, -1
	if p.p4Mechanics {
		wantVersion = 3 // P4 freezes source Scene as well as the existing rescue facts.
		if context.Scene == nil || int(*context.Scene) != c.Mission.Data.Scene {
			t.Fatal("P4 rescue result lost the actual source Scene owner")
		}
		frozenScene = int(*context.Scene)
	}
	t.Logf("reached rescue facts: context%d sourceScene%d frozenScene%d rescues%v actor1flags%02x", context.Version, c.Mission.Data.Scene, frozenScene, context.ReservedRescues, context.Actors[1].Flags)
	if context.Version != wantVersion || len(context.ReservedRescues) != 4 || context.ReservedRescues[0].Kind != 0x41 || context.ReservedRescues[0].Carrier != 0 || context.Actors[1].Flags&0x80 == 0 {
		t.Fatal("real carrier return did not export versioned source rescue ownership")
	}

	loopAcknowledge(t, c)
	committed := c.Store.State()
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, reopened.State()) {
		t.Fatal("reserve result durable restart changed state")
	}
	if err := c.commitDebriefResult(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committed, c.Store.State()) {
		t.Fatal("old pickup result awarded twice")
	}
}
