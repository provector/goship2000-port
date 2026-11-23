package combat

import (
	"encoding/binary"
	"testing"

	"gunship2000-assets/game/avionics"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	gamestate "gunship2000-assets/game/state"
)

// The original B writers intentionally do not add a relative OBJECT's H0:
// resident07E6:126A..12F3 and GPS4:10C4..118B. Thus two formation members
// with opposite lateral offsets and the same longitudinal offset can share
// one return point. Kind22:EA73 and Kind26:F512 do not add landing separation.
// This protects the source behavior; a landing-spacing policy is a new design.
func TestReturnHomeAliasingAndIndependentTouchdown(t *testing.T) {
	for _, tc := range []struct {
		name string
		own  [2][2]int32
		farp bool
		want [2]navigation.Point
	}{
		{"opposite_lateral_same_longitudinal", [2][2]int32{{-400, -200}, {400, -200}}, false, [2]navigation.Point{{H0: 0x40000, H1: 0x40000 - 200}, {H0: 0x40000, H1: 0x40000 - 200}}},
		{"distinct_longitudinal", [2][2]int32{{-400, -200}, {400, -400}}, false, [2]navigation.Point{{H0: 0x40000, H1: 0x40000 - 200}, {H0: 0x40000, H1: 0x40000 - 400}}},
		{"absolute_home_preserves_lateral", [2][2]int32{{0x40000 - 400, 0x40000 - 200}, {0x40000 + 400, 0x40000 - 200}}, false, [2]navigation.Point{{H0: 0x40000 - 400, H1: 0x40000 - 200}, {H0: 0x40000 + 400, H1: 0x40000 - 200}}},
		{"farp_toggle_shared_destination", [2][2]int32{{-400, -200}, {400, -400}}, true, [2]navigation.Point{{H0: 0x48000, H1: 0x48000}, {H0: 0x48000, H1: 0x48000}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := routeFixture(t)
			m := s.init
			base, farp := navigation.Point{H0: 0x40000, H1: 0x40000}, navigation.Point{H0: 0x48000, H1: 0x48000}
			binary.LittleEndian.PutUint32(m.Rows[0].Raw[2:], base.H0)
			binary.LittleEndian.PutUint32(m.Rows[0].Raw[6:], base.H1)
			var routes [5]navigation.Route
			var contacts [80]avionics.Contact
			var active [5]bool
			rows := make([][32]byte, len(m.Rows))
			for actor := 1; actor <= 2; actor++ {
				e := &m.Entities[actor]
				binary.LittleEndian.PutUint32(m.Rows[actor].Raw[2:], uint32(tc.own[actor-1][0]))
				binary.LittleEndian.PutUint32(m.Rows[actor].Raw[6:], uint32(tc.own[actor-1][1]))
				e.ControlRaw[1], e.ControlRaw[5] = 1, 0
				routes[actor] = navigation.Route{Raw: e.ControlRaw}
				if tc.farp {
					_ = routes[actor].SetPoint(1, base)
				}
				active[actor] = true
			}
			for i := range m.Rows {
				rows[i] = m.Rows[i].Raw
			}
			for actor := 1; actor <= 2; actor++ {
				home, err := navigation.ResolveHome(rows, routes[actor].Raw, actor)
				if err != nil {
					t.Fatal(err)
				}
				effects := navigation.ApplyOrder(&routes, active, actor, "b", base, home, farp, tc.farp)
				e := &m.Entities[actor]
				e.ControlRaw, e.Kind, e.Participation = routes[actor].Raw, effects[actor].Kind, 0
				if !effects[actor].ChangeKind || !effects[actor].ResetPhase || e.Kind != 0x22 || routes[actor].Point(1) != tc.want[actor-1] {
					t.Fatalf("actor%d B writer: home%+v route%+v effects%+v", actor, home, routes[actor], effects[actor])
				}
				if err := s.FinalizeDirectReturn(actor, &contacts); err != nil {
					t.Fatal(err)
				}
				// Both actors approach naturally; actor2 starts farther away.
				e.Pose = gamestate.InitializedPose{H0: tc.want[actor-1].H0, H1: tc.want[actor-1].H1 - uint32(6000+actor*1000), Height: 153}
			}
			in.LowLimit = 0
			in.Random = func() uint16 { t.Fatal("normal home arrival/descent added RNG"); return 0 }
			sys := flight.NewSystems([]int16{100, 100, 100, 100, 100}, []bool{true, true, true, true, true})
			var landed, idle [2]int
			for tick := 1; tick <= 1000 && (idle[0] == 0 || idle[1] == 0); tick++ {
				for actor := 1; actor <= 2; actor++ {
					e := &m.Entities[actor]
					oldKind := e.Kind
					var err error
					switch oldKind {
					case 0x22:
						err = s.advanceFollowerRoute(actor, in)
					case 0x26:
						err = s.advanceApproach(actor, in, &sys)
					case 0x20:
						err = s.advanceIdleFollower(actor)
					default:
						t.Fatalf("unexpected actor%d kind%02x", actor, oldKind)
					}
					if err != nil {
						t.Fatal(err)
					}
					if oldKind == 0x22 && e.Kind == 0x26 && (e.Pose.H0 != tc.want[actor-1].H0 || e.Pose.H1 != tc.want[actor-1].H1 || e.Participation != 1) {
						t.Fatal("arrival lost cached destination or inherited phase", actor, e.Pose, e.Participation)
					}
					if oldKind == 0x26 && e.Kind == 0x20 {
						landed[actor-1] = tick
						if e.Participation != 0 || e.ControlRaw[1] != 2 || e.Pose.Height != 0 {
							t.Fatal("touchdown successor", actor, e)
						}
					}
					if oldKind == 0x20 && e.Participation == 1 && idle[actor-1] == 0 {
						idle[actor-1] = tick
					}
				}
			}
			for actor := 1; actor <= 2; actor++ {
				e := m.Entities[actor]
				if landed[actor-1] == 0 || idle[actor-1] != landed[actor-1]+1 || e.Pose.H0 != tc.want[actor-1].H0 || e.Pose.H1 != tc.want[actor-1].H1 || e.Pose.Height != 2 || e.Kind != 0x20 || !e.Visible {
					t.Fatal("parked source aircraft not retained", actor, landed, idle, e)
				}
			}
			t.Logf("separate actor visits: touchdown=%v parked=%v endpoints=%+v", landed, idle, tc.want)
		})
	}
}
