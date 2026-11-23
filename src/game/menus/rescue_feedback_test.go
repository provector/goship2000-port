package menus

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestActualReservedPickupThanksPreservesOrdinaryCargo(t *testing.T) {
	for _, actor := range []int{1, 4} {
		for _, cargo := range []byte{0, 8, 16, 24} {
			t.Run(fmt.Sprintf("actor%d/cargo%02x", actor, cargo), func(t *testing.T) {
				c := developmentScenarioController(t, "n6-rescue")
				p := c.Rendering
				r := p.Flight
				m := p.Initialized
				// Labelled supplied ordinary upper flags match the recovered direct pickup
				// writer (which does not immediately increase mass). Real reserve mechanics
				// still create the survivor and its carrier; no pickup/result is prefilled.
				m.Stores.Ships[0].Flags = m.Stores.Ships[0].Flags&7 | cargo
				if actor == 4 {
					e, old := &m.Entities[4], m.Entities[1]
					e.Kind, e.Participation, e.Pose, e.RawMotion = 0x20, 1, old.Pose, 0
					e.ControlRaw[1], e.ControlRaw[5] = 1, 0
					binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
					e.DynamicRaw[0], e.DynamicRaw[1] = 0x20, 1
					binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 0)
					binary.LittleEndian.PutUint32(e.DynamicRaw[14:], e.Pose.H0)
					binary.LittleEndian.PutUint32(e.DynamicRaw[18:], e.Pose.H1)
					binary.LittleEndian.PutUint16(e.DynamicRaw[22:], e.Pose.Height)
					r.Systems.Fuel[1], r.Systems.Fuel[4] = 100, 0
					m.Stores.Ships[1].Fuel, m.Stores.Ships[4].Fuel = 100, 0
				}
				beforeFlags, beforeMass := m.Stores.Ships[0].Flags, r.MassTerm
				if err := c.Handle(Actions{Cancel: true}); err != nil {
					t.Fatal(err)
				}
				reserve := actor + 4
				for i := 0; i < 80 && m.Entities[reserve].Kind != 0x41; i++ {
					if err := c.AdvanceRendering(.12); err != nil {
						t.Fatal(err)
					}
				}
				if m.Entities[reserve].Kind != 0x41 || m.Entities[reserve].DynamicRaw[3] != 0 {
					t.Fatal("production source-positioned pickup absent")
				}
				found := false
				for _, row := range r.Notifications.Rows {
					if row[0] == 21 && int(row[1]) == actor && row[2] > 0 {
						found = true
					}
				}
				if !found {
					t.Fatal("actual rescued actor producer did not reach admitted thanks queue", r.Notifications)
				}
				if m.Stores.Ships[0].Flags != beforeFlags || r.MassTerm != beforeMass {
					t.Fatal("reserve pickup invented ordinary cargo/mass")
				}
				if p.Cockpit.Notifications == nil || p.Cockpit.Notifications.ObjectNames[actor] == "" {
					t.Fatal("named actor display owner missing")
				}
				// Ordinary GPS cargo inspection/drop does not alias reserve carrier state.
				if err := c.Handle(Actions{}); err != nil {
					t.Fatal(err)
				}
				if err := c.Handle(Actions{GPSOpen: true}); err != nil {
					t.Fatal(err)
				}
				if p.GPS == nil {
					t.Fatal("GPS unavailable")
				}
				if p.GPS.Panel.ShipFlags != beforeFlags {
					t.Fatal("GPS replaced ordinary hold with survivor")
				}
				if err := c.Handle(Actions{GPSKey: "c"}); err != nil {
					t.Fatal(err)
				}
				if m.Entities[reserve].Kind != 0x41 || m.Entities[reserve].DynamicRaw[3] != 0 || m.Stores.Ships[0].Flags&0xf8 != 0 {
					t.Fatal("ordinary drop lost survivor or retained cargo")
				}
			})
		}
	}
}

func TestReservedPickupVoluntaryEndDoesNotGrantCarrierReturn(t *testing.T) {
	c := developmentScenarioController(t, "n6-rescue")
	p := c.Rendering
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 80 && p.Initialized.Entities[5].Kind != 0x41; i++ {
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
	}
	if p.Initialized.Entities[5].Kind != 0x41 {
		t.Fatal("real pickup absent")
	}
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RenderRGBA(false); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{EndMission: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenQuit {
		t.Fatal("actual Alt+E confirmation missing", c.Screen)
	}
	if err := c.Handle(Actions{ConfirmQuit: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief == nil || c.Debrief.Pending == nil {
		t.Fatal("source result unavailable", c.Message)
	}
	f := c.Debrief.Pending.Proposal.Facts
	if f.SourceFlags&0x80 != 0 || f.ResultContext.Actors[1].Flags&0x80 != 0 {
		t.Fatal("voluntary ending invented carrier return", f.SourceFlags, f.ResultContext.Actors[1].Flags)
	}
}
