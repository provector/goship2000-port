package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestObjectiveSightingDoesNotDivertOrdinaryTroopRoute(t *testing.T) {
	for _, kind := range []byte{0x20, 0x21, 0x22, 0x23} {
		s, _ := routeFixture(t)
		e, cargo := &s.init.Entities[1], &s.init.Entities[10]
		e.Kind = kind
		e.Participation = 1
		cargo.Kind = 0x15
		cargo.Object.Raw[0] = 0x10
		before := *e
		if err := s.ContactOrder(1, 10, 0); err != nil {
			t.Fatal(err)
		}
		if *e != before {
			t.Fatalf("kind%02x ordinary objective sighting changed route/controller", kind)
		}
		if ns := events(s, "notification"); len(ns) != 1 || ns[0].Notification != 11 || ns[0].Actor != 1 {
			t.Fatal(ns)
		}
	}
}

func TestApproachAndTransferRemainSeparateSourceBoundaries(t *testing.T) {
	for _, role := range []int{0, 1} {
		for _, goal := range []uint16{1, 2} {
			for _, metric := range []uint32{127, 128} {
				for _, kind := range []byte{0x20, 0x21, 0x22, 0x23} {
					s, in := routeFixture(t)
					m := s.init
					e := &m.Entities[1]
					e.Kind = kind
					e.Participation = 1
					e.ControlRaw[1] = 2
					e.ControlRaw[5] = 2
					e.Pose.Height = 64
					m.Entities[2].Kind = 0x20
					for i := 0; i < 5; i++ {
						m.Stores.Ships[i].Flags = 0
					}
					m.Stores.Ships[1].Flags = 6
					if goal == 1 {
						m.Stores.Ships[1].Flags |= 0x18
					}
					r := &m.Targets.Records[role+2]
					binary.LittleEndian.PutUint16(r[32:], goal)
					binary.LittleEndian.PutUint16(r[52:], 100)
					binary.LittleEndian.PutUint32(r[34:], e.Pose.H0+metric*16)
					binary.LittleEndian.PutUint32(r[38:], e.Pose.H1)
					in.Random = func() uint16 { t.Fatal("objective phase consumed RNG"); return 0 }
					s.dispatchFlightObjective(role, in, &primaryScanTrace{})
					want := metric == 127 && (kind == 0x20 || kind == 0x21)
					if (e.Kind == 0x26 && e.ControlRaw[1] == 7) != want {
						t.Fatalf("role%d goal%d metric%d kind%02x approach%v: %+v", role, goal, metric, kind, want, e)
					}
					if binary.LittleEndian.Uint16(r[32:]) != goal || m.ObjectiveAssignments[role] != 0 {
						t.Fatal("approach fabricated transfer")
					}
				}
			}
		}
	}
}

// This goes past the site before the final waypoint: mere proximity in route
// mode does not land. The same source route ending nearby holds and then lands.
func TestOrderedNearPassVersusNearbyFinalWaypoint(t *testing.T) {
	for _, goal := range []uint16{1, 2} {
		for _, finalNear := range []bool{false, true} {
			s, in := routeFixture(t)
			m := s.init
			e := &m.Entities[1]
			start := e.Pose
			site := start
			site.H1 += 12000
			endpoint := site
			if finalNear {
				endpoint.H1 += 640
			} else {
				endpoint.H1 += 40000
			}
			proficiency := byte(127)
			m.Stores.Ships[1].Proficiency = &proficiency
			for i := 0; i < 5; i++ {
				m.Stores.Ships[i].Flags = 0
			}
			m.Stores.Ships[1].Flags = 6
			if goal == 1 {
				m.Stores.Ships[1].Flags |= 0x18
			}
			r := &m.Targets.Records[2]
			binary.LittleEndian.PutUint16(r[32:], goal)
			binary.LittleEndian.PutUint32(r[34:], site.H0)
			binary.LittleEndian.PutUint32(r[38:], site.H1)
			cargo := &m.Entities[10]
			cargo.Kind = 0x15
			cargo.Participation = 1
			cargo.Object.Raw[0] = 0x10
			cargo.Pose = site
			if !finalNear {
				// A genuine intermediate waypoint at the site cannot invoke
				// last-point target avoidance while a later point exists.
				binary.LittleEndian.PutUint32(e.ControlRaw[10:], site.H0)
				binary.LittleEndian.PutUint32(e.ControlRaw[14:], site.H1)
				binary.LittleEndian.PutUint32(e.ControlRaw[20:], endpoint.H0)
				binary.LittleEndian.PutUint32(e.ControlRaw[24:], endpoint.H1)
			} else {
				binary.LittleEndian.PutUint32(e.ControlRaw[10:], endpoint.H0)
				binary.LittleEndian.PutUint32(e.ControlRaw[14:], endpoint.H1)
			}
			var contacts [80]avionics.Contact
			if err := s.FinalizeFollowerRoutes(in.Random, &contacts); err != nil {
				t.Fatal(err)
			}
			sys := flight.NewSystems([]int16{100, 100, 100, 100, 100}, []bool{true, true, true, true, true})
			sawNearWhileRouting := false
			for steps := 0; steps < 5000 && !s.Primary.Achieved; steps++ {
				if _, err := s.AdvanceFollowers(in, &sys); err != nil {
					t.Fatal(err)
				}
				if gamestate.ObjectiveDistance(*r, e.Pose) < 128 && (e.Kind == 0x22 || e.Kind == 0x23) {
					sawNearWhileRouting = true
				}
				s.AdvanceObjectives(in)
				if !finalNear && e.Kind == 0x20 && e.ControlRaw[1] == 2 {
					break
				}
			}
			if s.Primary.Achieved != finalNear {
				t.Fatalf("goal%d finalNear%v achieved%v e%+v", goal, finalNear, s.Primary.Achieved, *e)
			}
			if !sawNearWhileRouting {
				t.Fatalf("goal%d finalNear%v route did not actually pass inside approach distance", goal, finalNear)
			}
			if sys.UnsupportedFollowerEmptyBurn != 0 || m.FollowerLimits[1] != "" {
				t.Fatal(m.FollowerLimits)
			}
		}
	}
}

func TestGPSFinalPointSiteCorrectionStrict800AndTrueQuery(t *testing.T) {
	for _, distance := range []uint32{0x7ff, 0x800} {
		for _, goal := range []uint16{1, 2} {
			s, in := routeFixture(t)
			m := s.init
			e := &m.Entities[1]
			proficiency := byte(127)
			m.Stores.Ships[1].Proficiency = &proficiency
			x, z := e.Pose.H0+50000, e.Pose.H1+50000
			r := &m.Targets.Records[2]
			binary.LittleEndian.PutUint16(r[32:], goal)
			binary.LittleEndian.PutUint32(r[34:], x)
			binary.LittleEndian.PutUint32(r[38:], z)
			// The separate approximate search/display words must not own it.
			binary.LittleEndian.PutUint32(r[42:], x+65536)
			binary.LittleEndian.PutUint32(r[46:], z+65536)
			binary.LittleEndian.PutUint32(e.ControlRaw[10:], x+distance*16)
			binary.LittleEndian.PutUint32(e.ControlRaw[14:], z)
			draws := 0
			in.Random = func() uint16 { draws++; return 0 }
			var contacts [80]avionics.Contact
			if err := s.FinalizeFollowerRoutes(in.Random, &contacts); err != nil {
				t.Fatal(err)
			}
			if distance == 0x7ff {
				if draws != 0 || binary.LittleEndian.Uint32(e.ControlRaw[40:]) != x-128 || binary.LittleEndian.Uint32(e.ControlRaw[44:]) != z-128 {
					t.Fatal("inside final-point correction", goal, draws, e.ControlRaw)
				}
			} else if draws != 4 || binary.LittleEndian.Uint32(e.ControlRaw[40:]) != x+distance*16 {
				t.Fatal("equal boundary must retain ordinary variation", goal, draws, e.ControlRaw)
			}
		}
	}
}
