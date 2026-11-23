package flightcontrol

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
)

// ApplyHeight uses the original Easy thrust curve and velocity reader. Neutral
// arrests motion before capturing AGL, avoiding a forced return through the
// release height. It writes collective only: source inertia/contact/ceiling and
// power remain authoritative. The old experimental v7 reader stays separate.
func (s *LiveState) ApplyHeight(in LiveInput, f *flight.State, params flight.Parameters, easy, manual bool) {
	s.applyHeightCurve(in, f, params, easy, manual, nil, nil)
}

// ApplyHeightDamage retains the accepted assist and its original Easy hover
// datum, while predicting the actual source602/607/609 engine-reduced curve.
// The caller selects the R231 interpretation; older films use ApplyHeight.
func (s *LiveState) ApplyHeightDamage(in LiveInput, f *flight.State, params flight.Parameters, easy, manual bool, damage [10]byte) {
	s.applyHeightCurve(in, f, params, easy, manual, &damage, nil)
}

func (s *LiveState) applyHeightCurve(in LiveInput, f *flight.State, params flight.Parameters, easy, manual bool, damage *[10]byte, power *flight.Input) {
	if in.Mode != "relative_height" || in.Slider || !in.Bound[2] || !easy {
		s.HeightKnown, s.HeightArresting = false, false
		if in.Mode == "relative_height" && !in.Slider && in.Bound[2] && !easy && !manual {
			s.CollectiveRemainder += int32(in.Axes[2])
			f.Collective = max(0, min(31, f.Collective+int16(s.CollectiveRemainder/127)))
			s.CollectiveRemainder %= 127
		}
		return
	}
	if manual || in.Digital[2][0] || in.Digital[2][1] || f.HeightControl != 0 || f.Hover != 0 || f.Rotor == 0 || f.ValidContact && in.Axes[2] <= 0 {
		s.HeightKnown, s.HeightArresting = false, false
		return
	}
	hover := int32(int16(binary.LittleEndian.Uint16(params.Raw[0x30:])))
	if !s.HeightKnown {
		s.HeightTarget = f.RadarAltitude
		s.HeightKnown = true
		s.HeightMoving = false
		s.HeightArresting = f.Vertical != 0
	}
	want := int32(0)
	if axis := int32(in.Axes[2]); axis != 0 {
		want = axis * 8 / 127
		if want == 0 {
			if axis < 0 {
				want = -1
			} else {
				want = 1
			}
		}
		s.HeightMoving = true
		s.HeightArresting = false
	} else {
		if s.HeightMoving {
			s.HeightMoving = false
			s.HeightArresting = true
			s.HeightTarget = f.RadarAltitude
		}
		if s.HeightArresting && f.Vertical == 0 && (int32(f.Thrust)-hover)>>5 == 0 {
			s.HeightTarget = f.RadarAltitude
			s.HeightArresting = false
		}
		if !s.HeightArresting {
			want = max(-4, min(4, int32(s.HeightTarget-f.RadarAltitude)/6))
		}
	}
	// In the next E0 source update: thrust += (STAT[collective]-thrust)>>1;
	// Easy then approaches (thrust-STAT[24])>>5 by at most2. Prefer that
	// exact integer rate band, not a guessed proportional collective. Limit
	// actuation to2 source ordinals per logical input tick. During arrest a
	// cost penalty prevents requesting an opposite-sign vertical rate.
	target := hover + want*32 + 16
	best := f.Collective
	bestCost := int64(1 << 62)
	for c := max(0, f.Collective-2); c <= min(31, f.Collective+2); c++ {
		curve := int32(int16(binary.LittleEndian.Uint16(params.Raw[int(c)*2:])))
		if power != nil {
			curve = int32(power.EngineThrust(int16(curve), int16(binary.LittleEndian.Uint16(params.Raw[62:]))))
		} else if damage != nil {
			if damage[9] != 0 || damage[2] == 2 && damage[7] == 2 {
				curve = 0
			} else {
				if damage[2] == 2 || damage[7] == 2 {
					curve -= curve >> 2
				}
				if damage[2] == 1 || damage[7] == 1 {
					curve -= curve >> 4
				}
			}
		}
		next := int32(f.Thrust) + ((curve - int32(f.Thrust)) >> 1)
		rate := (next - hover) >> 5
		cost := int64(abs32(rate-want))*65536 + int64(abs32(next-target))
		if s.HeightArresting && (f.Vertical > 0 && rate < 0 || f.Vertical < 0 && rate > 0 || f.Vertical == 0 && rate != 0) {
			cost += 1 << 40
		}
		if cost < bestCost {
			best, bestCost = c, cost
		}
	}
	f.Collective = best
}
