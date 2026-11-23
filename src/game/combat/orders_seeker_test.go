package combat

import (
	"reflect"
	"testing"

	gamestate "gunship2000-assets/game/state"
)

func TestAvoidanceFallbackIsLocalAndKnownWriterWins(t *testing.T) {
	s, _ := routeFixture(t)
	scratchRegions(s)
	s.SetDispatcherScratchEnabled(true)
	s.SetSafeFlightOrdersEnabled(true)
	a := s.init.Entities[1].Pose
	a.Height = 99
	b := a
	b.H0 += 256
	got, err := s.followerRouteBlocked(a, b, 3000)
	if err != nil || !got || s.dispatcherScratchKnown {
		t.Fatal("unknown body operand not local", got, err, s.dispatcherScratchKnown)
	}
	a.Height = 100
	got, err = s.followerRouteBlocked(a, b, 3000)
	if err != nil || got || s.dispatcherScratchKnown {
		t.Fatal("another actor inherited fallback", got, err)
	}
	s.retainDispatcherHeight(99)
	got, err = s.followerRouteBlocked(a, b, 3000)
	if err != nil || !got || s.dispatcherScratchHeight != 99 {
		t.Fatal("body replaced reached writer", got, err)
	}
	s.dispatcherScratchKnown = false
	for _, height := range []uint16{99, 100, 32767, 0x8000} {
		a.Height = height
		got, err = s.formationCornerBlocked(a, 1500)
		q := deathQuery(a)
		want, e := s.assets.ContactHeight(s.init.World, q, s.deathQueryObjects())
		if err != nil || e != nil || got != (1500 < want) || s.dispatcherScratchKnown {
			t.Fatal("corner signed/body operand", height, got, want, err, e)
		}
	}
}

func TestDisengagePreservesHeadingAndOriginalCountdown(t *testing.T) {
	for _, safe := range []bool{false, true} {
		s, in := routeFixture(t)
		s.SetSafeFlightOrdersEnabled(safe)
		e, child := &s.init.Entities[3], &s.init.Entities[4]
		e.Kind, e.Participation, e.ControlRaw[1], e.ControlRaw[3] = 0x25, 1, 3, 0
		e.Pose.Angles[1], e.Pose.Angles[2] = 0xc000, 0x800
		child.ControlRaw[1], child.ControlRaw[5], child.ControlRaw[3] = 1, 3, 1
		s.init.Rows[4].Raw[10] |= 0x20
		in.E0 = false
		err := s.advanceOrphan(3, in)
		if !safe {
			if err == nil {
				t.Fatal("historical guard changed")
			}
			continue
		}
		if err != nil || e.Pose.Angles[1] != 0xc000 || e.Pose.Angles[2] != 0 || e.DynamicRaw[3] != 60 || e.ControlRaw[3] != 0 || child.ControlRaw[3] != 1 {
			t.Fatal("safe disengage", err, *e, *child)
		}
		child.ControlRaw[3] = 0
		e.DynamicRaw[3] = 1
		in.E0 = true
		if err = s.advanceOrphan(3, in); err != nil || e.DynamicRaw[3] != 0 || e.Kind != 0x25 {
			t.Fatal("timeout decremented/transitioned early", err, *e)
		}
		in.E0 = false
		if err = s.advanceOrphan(3, in); err != nil || e.Kind != 0x20 || e.Participation != 0 || e.ControlRaw[1] != 2 {
			t.Fatal("zero-before-E0 timeout", err, *e)
		}
	}
}

func TestFreshSeekerAndActualReadBoundary(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		for _, yaw := range []uint16{0, 0x800, 0x4000, 0x4001} {
			s, in := incomingFixture(t, 3)
			if fresh {
				s.InitializeFreshSeeker()
			}
			p := incomingAtTarget(s, 0x43, 0)
			p.Pose.H1 -= 4096
			p.Speed, p.Parameter, p.Pose.Angles[1] = 1, 64, yaw
			if yaw >= 0x4000 {
				p.Pose.H0++
			}
			s.init.Entities[1].Participation = 0
			draws := 0
			in.Random = func() uint16 { draws++; return 7 }
			if err := s.SupplyIncoming(20, p, false); err != nil {
				t.Fatal(err)
			}
			err := s.Step(in)
			if !fresh {
				if err == nil {
					t.Fatal("historical unknown changed", yaw)
				}
			} else if err != nil || !s.incomingAimKnown || s.incomingAimPose != (gamestate.InitializedPose{}) || draws != 0 {
				t.Fatal("fresh CRT seeker", yaw, err, s.incomingAimPose, draws)
			}
		}
	}
	// An incomplete foreign epoch is guarded only when the pitch actually reads
	// its height; oblique/expiry branches do not need invented state.
	for _, yaw := range []uint16{0, 0x800, 0x4000, 0x4001} {
		s, in := incomingFixture(t, 3)
		s.conditionalSeekerReads = true
		p := incomingAtTarget(s, 0x43, 0)
		p.Pose.H1 -= 4096
		p.Speed, p.Pose.Angles[1] = 1, yaw
		if yaw >= 0x4000 {
			p.Pose.H0++
		}
		s.init.Entities[1].Participation = 0
		if err := s.SupplyIncoming(20, p, false); err != nil {
			t.Fatal(err)
		}
		err := s.Step(in)
		if (err != nil) != (yaw < 0x800) {
			t.Fatal(yaw, err)
		}
	}
	s, in := incomingFixture(t, 3)
	s.InitializeFreshSeeker()
	p := incomingAtTarget(s, 0x83, 0)
	p.Pose.H1 -= 4096
	p.Speed = 1
	if err := s.SupplyIncoming(20, p, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	want := s.incomingAimPose
	s.init.Entities[1].Participation = 0
	s.Slots[20].Flags = 0x43
	if err := s.Step(in); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, s.incomingAimPose) {
		t.Fatal("later visit reset shared writer")
	}
}
