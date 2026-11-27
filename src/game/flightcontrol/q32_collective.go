package flightcontrol

import "gunship2000-assets/game/flight"

// ApplyQ32 preserves the resolved tick operand. Easy height control owns
// collective for either physical mechanism, rather than silently treating a
// slider's height mode as manual position-delta collective.
func (s *LiveState) ApplyQ32(in LiveInput, a *Axes, c int16, manual, easy bool) int16 {
	if easy && in.Mode == "relative_height" {
		copy := in
		copy.Bound[2] = false
		return s.Apply(copy, a, c, manual)
	}
	return s.Apply(in, a, c, manual)
}
func sliderHeightAxis(v uint16) int16 {
	x := int32(v)*2 - 65535
	if abs32(x) <= 3277 {
		return 0
	} // 5% signed-axis preview dead zone.
	if x > 0 {
		return int16((x - 3277) * 127 / (65535 - 3277))
	}
	return -int16((-x - 3277) * 127 / (65535 - 3277))
}

// ApplyHeightQ32 shares the accepted BDA arrest-first owner. Slider midpoint
// is neutral in this mode only. Entry/reconnect/Resume/keyboard takeover require
// reaching neutral before accepting deflection, so a parked lever cannot jump.
func (s *LiveState) ApplyHeightQ32(in LiveInput, f *flight.State, p flight.Parameters, easy, manual bool) {
	s.applyHeightQ32Curve(in, f, p, easy, manual, nil, nil)
}

// ApplyHeightQ32Damage is the R231 launch-selected reader. Healthy and all old
// profiles retain the accepted arrest/hold response; damage changes prediction,
// never the underlying source flight equations or the saved collective mode.
func (s *LiveState) ApplyHeightQ32Damage(in LiveInput, f *flight.State, p flight.Parameters, easy, manual bool, damage [10]byte, avionics byte) {
	if heightAssistUnavailable(f, damage, avionics) {
		s.HeightKnown, s.HeightArresting = false, false
		return // ApplyQ32Damage owns the manual relative input, once.
	}
	s.applyHeightQ32Curve(in, f, p, easy, manual, &damage, nil)
}

// ApplyHeightWithPower uses the same thrust curve as flight. Only a fresh
// deflection after axis pickup can release a conflicting hover/height hold.
func (s *LiveState) ApplyHeightWithPower(in LiveInput, f *flight.State, p flight.Parameters, easy, manual bool, power flight.Input, avionics byte) {
	if heightAssistUnavailable(f, power.Damage, avionics) || power.FuelSupplyFailed {
		s.HeightAxisSampled = false
		return
	}
	s.applyHeightQ32Curve(in, f, p, easy, manual, &power.Damage, &power)
}

func (s *LiveState) applyHeightQ32Curve(in LiveInput, f *flight.State, p flight.Parameters, easy, manual bool, damage *[10]byte, power *flight.Input) {
	if in.Mode == "relative_height" && in.Slider && easy && in.Bound[2] {
		axis := sliderHeightAxis(in.Lever)
		if manual || in.Digital[2][0] || in.Digital[2][1] {
			s.HeightSliderReady = false
		}
		if axis == 0 && !manual && !in.Digital[2][0] && !in.Digital[2][1] {
			s.HeightSliderReady = true
		}
		if !s.HeightSliderReady {
			s.HeightKnown = false
			s.HeightAxisSampled = false
			return
		}
		in.Axes[2] = axis
		in.Slider = false
	}
	if power != nil {
		admitted := in.Mode == "relative_height" && in.Bound[2] && !manual && easy
		if admitted {
			axis := in.Axes[2]
			moved := abs32(int32(axis)-int32(s.HeightAxisAnchor)) >= 4
			if power.IndependentHover && f.Hover != 0 && !s.HeightKnown && !(s.HeightAxisSampled && moved && axis != 0) {
				if !s.HeightAxisSampled || axis == 0 {
					s.HeightAxisAnchor = axis
				}
				s.HeightAxisSampled = true
				return
			}
			if s.HeightAxisSampled && moved && axis != 0 {
				f.HeightControl = 0
				if !power.IndependentHover {
					f.Hover = 0
				}
			}
			if !s.HeightAxisSampled || moved || axis == 0 {
				s.HeightAxisAnchor = axis
			}
		}
		s.HeightAxisSampled = admitted
	}
	s.applyHeightCurve(in, f, p, easy, manual, damage, power)
}
