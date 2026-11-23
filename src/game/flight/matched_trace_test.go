package flight

import (
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/world"
	"testing"
)

// Independent checkpoints below are source29xx/2b25/2b63 operands and Step41
// hand cases. Full Step then records real force/contact successors; those later
// values are diagnostic evidence, not promoted to self-generated golden data.
func TestProductionNavigationMatchedCheckpointsAndTraces(t *testing.T) {
	p, base := sourceFixture(t)
	for _, tc := range []struct {
		name         string
		ridge        int16
		phase        uint16
		heading      uint16
		near         uint32
		final        bool
		manual       int16
		damage       bool
		wantHeading  uint16
		checkHeading bool
		wantIndex    int8
		wantHold     int16
	}{
		{"flat-route", 0, 1, 1024, 20000, false, 0, false, 986, true, 1, 1},
		{"ridge-left-phase", 700, 1, 0, 20000, false, 0, false, 65229, true, 1, 1},
		{"translated-ridge-right-phase", 700, 0x401, 0, 20000, false, 0, false, 307, true, 1, 1},
		{"descending-support", 200, 1, 0, 20000, false, 0, false, 0, true, 1, 1},
		{"waypoint-approach", 0, 1, 0, 400, false, 0, false, 0, true, 2, 1},
		{"past-waypoint", 0, 1, 0, 400, true, 0, false, 0, true, 0, 0},
		{"manual32", 0, 1, 0, 20000, false, 32, false, 0, false, 1, 1},
		{"manual33", 0, 1, 0, 20000, false, 33, false, 0, false, 1, 0},
		{"disabled609", 0, 1, 0, 20000, false, 0, true, 0, false, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, z := uint32(200000), uint32(200000)
			if tc.name == "translated-ridge-right-phase" {
				x += 32768
				z += 16384
			}
			s := State{Pose: Pose{H0: x, H1: z, Height: 600, Angles: [3]uint16{0, tc.heading, 0}}, RadarAltitude: 598, Rotor: 1, Collective: 16, HeightControl: 1, HeightTarget: 600, CruiseSpeed: 64, IndicatedSpeed: 64, WaypointIndex: 1, LowLimit: 128, Power: 1024, Thrust: int16(binary.LittleEndian.Uint16(p.Raw[32:]))}
			s.Waypoints[1] = Waypoint{H0: x, H1: z + tc.near}
			if !tc.final {
				s.Waypoints[2] = Waypoint{H0: x + 10000, H1: z + 20000}
			}
			q := base
			var samples []Pose
			rng := uint16(1)
			draws := 0
			q.Height = func(v Pose) (int16, error) {
				samples = append(samples, v)
				if int32(v.H1-z) > 512 {
					return tc.ridge, nil
				}
				return 0, nil
			}
			in := Input{DF: 6, Phase: tc.phase, Switches: 4, X: tc.manual, Random: func() uint16 {
				draws++
				if rng&1 != 0 {
					rng = rng>>1 ^ 0xb400
				} else {
					rng >>= 1
				}
				return rng
			}}
			if tc.damage {
				in.Damage[9] = 1
			}
			for step := 0; step < 6; step++ {
				in.Phase = tc.phase + uint16(step)
				in.E0 = step%3 != 2
				prior := s
				desired := prior.Pose.Angles[1]
				if prior.WaypointIndex > 0 {
					w := prior.Waypoints[prior.WaypointIndex]
					desired = world.SourceAngle(int16(int32(prior.Pose.H0)>>4)-int16(int32(w.H0)>>4), int16(int32(w.H1)>>4)-int16(int32(prior.Pose.H1)>>4), base.Angles)
					if tc.ridge > prior.Pose.Height+16 && abs(int16(prior.Pose.Angles[1]-desired)) < 0x4000 {
						if in.Phase&0x400 == 0 {
							desired += 0xe000
						} else {
							desired += 0x2000
						}
					}
				}
				samples = nil
				if err := Step(&s, p, in, q); err != nil {
					t.Fatal(err)
				}
				if step == 0 {
					if tc.checkHeading && s.Pose.Angles[1] != tc.wantHeading {
						t.Fatalf("first source steering divergence: %04x want%04x", s.Pose.Angles[1], tc.wantHeading)
					}
					if s.WaypointIndex != tc.wantIndex || s.HeightControl != tc.wantHold {
						t.Fatal("first source route/hold divergence", s.WaypointIndex, s.HeightControl, tc.wantIndex, tc.wantHold)
					}
				}
				raw, err := json.Marshal(struct {
					Name           string
					Step           int
					Before, After  State
					Input          any
					HeightQueries  []Pose
					DesiredHeading uint16
					RNG            uint16
					RNGDraws       int
				}{tc.name, step, prior, s, map[string]any{"DF": in.DF, "Phase": in.Phase, "E0": in.E0, "X": in.X, "Switches": in.Switches, "Damage": in.Damage}, append([]Pose(nil), samples...), desired, rng, draws})
				if err != nil {
					t.Fatal(err)
				}
				t.Log("NAV_TRACE", string(raw))
			}
		})
	}
	// No-route hold follows the recovered supplement change, not a route planner.
	s := State{Pose: Pose{H0: 200000, H1: 200000, Height: 600}, RadarAltitude: 598, Rotor: 1, HeightControl: 1, HeightTarget: 600, CruiseSpeed: 64, IndicatedSpeed: 64, Collective: 16}
	if err := Step(&s, p, Input{DF: 6, Phase: 4, E0: true}, base); err != nil {
		t.Fatal(err)
	}
	if s.WaypointIndex != 0 || s.HeightControl != 1 || s.Pose.Angles[1] != 0 || s.Pose.Height != 600 {
		t.Fatal("no-route source hold divergence", s)
	}
}
