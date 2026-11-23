package menus

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// Find an actually negative support region in the admitted world, inside the
// source flight bounds and clear of placed model support. This is supplied
// touchdown state, not an observation of Matt's unrecorded water location.
func waterPose(t *testing.T, p *RenderingPreview) flight.Pose {
	t.Helper()
	w := p.Renderer.Assets.Worlds[p.Initialized.World]
	for i := range w.Grid {
		x, z := uint32(i%64*8192+4096), uint32((63-i/64)*8192+4096)
		if x < 0x4000 || x > 0x7bfff || z < 0x4000 || z > 0x7bfff {
			continue
		}
		v := world.QueryPose{H0: x, H1: z, Height: 2}
		h, err := p.Renderer.Assets.ContactHeight(p.Initialized.World, v, p.Initialized.ContactObjects())
		if err != nil {
			t.Fatal(err)
		}
		if h != -1 {
			continue
		}
		flat := true
		for _, d := range [][2]int32{{-48, 0}, {48, 0}, {0, -48}, {0, 48}} {
			q := v
			q.H0 += uint32(d[0])
			q.H1 += uint32(d[1])
			a, e := p.Renderer.Assets.ContactHeight(p.Initialized.World, q, p.Initialized.ContactObjects())
			if e != nil {
				t.Fatal(e)
			}
			flat = flat && a == -1
		}
		if flat {
			return flight.Pose{H0: x, H1: z, Height: 2}
		}
	}
	t.Fatal("no actual negative water footprint in admitted world")
	return flight.Pose{}
}

func TestPreparedWaterTouchdownReachesActualResult(t *testing.T) {
	for _, family := range []string{"land", "carrier"} {
		t.Run(family, func(t *testing.T) {
			c := mastFixture(t, 0, family)
			// The retained I1 helper deliberately loads only render resources.
			// Admit the real, already-prepared debrief owner before the supplied
			// touchdown; do not turn a missing test prerequisite into a pass.
			complete := *c.Office.Assets
			complete.Debrief = renderingAssets.Debrief
			c.Office.Assets = &complete
			if err := c.beginSortieResult(c.Mission.Final); err != nil {
				t.Fatal(err)
			}
			if c.Mission.ResultLaunch == nil {
				t.Fatal(c.Mission.ResultUnavailable)
			}
			jFinishFade(t, c)
			p, r := c.Rendering, c.Rendering.Flight
			before := c.Store.State()
			r.State.Pose = waterPose(t, p)
			r.State.Rotor, r.State.Collective, r.State.Thrust, r.State.Vertical = 1, 0, 0, -4
			r.State.Hover, r.State.HeightControl, r.State.FlightStage, r.State.RadarAltitude = 0, 0, 0, 1
			r.State.History0, r.State.History1 = [4]int16{}, [4]int16{}
			for n := 0; n < 64 && c.Screen == ScreenScene; n++ {
				if err := c.advanceFlight(.1); err != nil {
					t.Fatal(err)
				}
				if c.Screen == ScreenScene {
					c.Frame(false)
					c.MarkScenePresented()
				}
			}
			if r.State.Terminal == nil || r.State.Terminal.SourceCode != 8 || r.State.Terminal.Status != 0x10 || c.Screen != ScreenDebrief {
				t.Fatal("actual water contact did not reach source terminal/result", family, p.Initialized.World, r.State.Terminal, c.Screen, r.Failure, c.Message)
			}
			proposal := c.Debrief.Pending.Proposal
			if !proposal.Facts.PlayerTerminal || proposal.Facts.SourceFlags&0x10 == 0 {
				t.Fatal("rough source facts omitted", proposal.Facts)
			}
			training := c.Mission.Data.Inputs.MissionType == gamestate.MissionTraining
			if training {
				if proposal.Result.Loss || proposal.AcceptedPilot.Status == gamestate.StatusKIA || proposal.AcceptedPilot.Status == gamestate.StatusMIA || proposal.Score.CareerApplied {
					t.Fatal("Training result protection lost", proposal.Result, proposal.AcceptedPilot.Status)
				}
			} else if !proposal.Result.Loss || proposal.AcceptedPilot.Status != gamestate.StatusKIA {
				t.Fatal("ordinary water loss missing", proposal.Result, proposal.AcceptedPilot.Status)
			}
			stage := c.Store.State()
			before.Revision = stage.Revision
			if !reflect.DeepEqual(before, stage) {
				t.Fatal("career changed before final acknowledgment")
			}
			t.Logf("prepared%s world%s water%+v switches%04x source%02x normalized%02x loss%v status%s career_applied%v", family, p.Initialized.World, r.State.Pose, p.Initialized.Switches, proposal.Facts.SourceFlags, proposal.Score.NormalizedFlags, proposal.Result.Loss, proposal.AcceptedPilot.Status, proposal.Score.CareerApplied)
			loopAcknowledge(t, c)
			committed, _ := json.Marshal(c.Store.State())
			again, e := gamestate.OpenSaveStore(c.Store.Directory, "no-original")
			if e != nil {
				t.Fatal(e)
			}
			if e = again.CommitSortie(proposal.Facts.SortieID); e != nil {
				t.Fatal(e)
			}
			restarted, _ := json.Marshal(again.State())
			if !bytes.Equal(committed, restarted) {
				t.Fatal("water result restart/duplicate acknowledgment changed state")
			}
		})
	}
}
