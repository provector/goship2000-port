package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/replay"
	"testing"
)

// A supplied short approach within a real generated five-aircraft mission.
// Ordinary flight ticks, dispatch, the player ending and result assembly follow.
func TestSectionHomeBeforePlayerDebrief(t *testing.T) {
	c := mastProfileFixture(t, 0, "pass", replay.R86Compatibility)
	jFinishFade(t, c)
	c.AbandonFilmRecording()
	p := c.Rendering
	m := p.Initialized
	if err := p.wingmanOrder(3, "b", false); err != nil {
		t.Fatal(err)
	}
	if err := p.Flight.Combat.FinalizeDirectReturn(3, &p.Flight.Avionics.Contacts); err != nil {
		t.Fatal(err)
	}
	p.syncFollowerNavigation()
	e := &m.Entities[3]
	x, z := binary.LittleEndian.Uint32(e.ControlRaw[40:]), binary.LittleEndian.Uint32(e.ControlRaw[44:])
	const edge = 0x80000
	e.Pose.H0, e.Pose.H1 = x, z
	switch {
	case x == 0:
		e.Pose.H0 = 2500
		e.Pose.Angles[1] = 0x4000
	case x == edge:
		e.Pose.H0 = edge - 2500
		e.Pose.Angles[1] = 0xc000
	case z == 0:
		e.Pose.H1 = 2500
		e.Pose.Angles[1] = 0x8000
	case z == edge:
		e.Pose.H1 = edge - 2500
		e.Pose.Angles[1] = 0
	default:
		t.Fatalf("fixture lacks derived gate (%d,%d)", x, z)
	}
	child := &m.Entities[4]
	child.Pose = e.Pose
	if child.ControlRaw[5] != 3 || child.Kind != 0x21 {
		t.Fatal("not a formed light section", child.ControlRaw, child.Kind)
	}
	e.Pose.Height, child.Pose.Height = 600, 600
	for n := 0; n < 800 && child.Kind != 0x41; n++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
	}
	if e.Kind != 0x41 || child.Kind != 0x41 {
		t.Fatal("section did not clear corridor", e.Kind, child.Kind, e.Pose, child.Pose, m.FollowerLimits)
	}
	for _, a := range []Actions{{}, {EndMission: true}, {}, {ConfirmQuit: true}} {
		if err := c.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenDebrief {
		t.Fatal(c.Screen, c.Message)
	}
	facts := c.Debrief.Pending.Proposal.Facts
	for _, row := range []int{3, 4} {
		a := facts.ResultContext.Actors[row]
		if a.Kind != 0x41 || a.Flags&0x80 == 0 {
			t.Fatal("source result lacks actual corridor return", row, a.Kind, a.Flags)
		}
	}
}
