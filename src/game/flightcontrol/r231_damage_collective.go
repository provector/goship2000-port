package flightcontrol

import "gunship2000-assets/game/flight"

func heightAssistUnavailable(f *flight.State, damage [10]byte, avionics byte) bool {
	return f.Rotor == 0 || damage[9] != 0 || avionics&3 == 2
}

// ApplyQ32Damage selects the R231 input policy. Relative Height remains the
// saved preference. If source power/rotor or critical avionics makes its assist
// unavailable, the pilot retains existing manual Relative control. A centered
// stick must rearm at neutral; a slider starts from its current parked anchor.
// This changes no flight state except the returned collective input, and uses
// neither wall time nor RNG. Older launch interpretations use ApplyQ32.
func (s *LiveState) ApplyQ32Damage(in LiveInput, a *Axes, f *flight.State, manual, easy bool, damage [10]byte, avionics byte) int16 {
	return s.ApplyCollectiveWithSupply(in, a, f, manual, easy, damage, avionics, false)
}

func (s *LiveState) ApplyCollectiveWithSupply(in LiveInput, a *Axes, f *flight.State, manual, easy bool, damage [10]byte, avionics byte, failedSupply bool) int16 {
	if in.Epoch != s.Epoch {
		*s = LiveState{Epoch: in.Epoch}
	}
	fallback := easy && in.Mode == "relative_height" && (failedSupply || heightAssistUnavailable(f, damage, avionics))
	if fallback != s.HeightManualFallback {
		s.HeightManualFallback = fallback
		s.HeightManualReady = false
		s.HeightKnown, s.HeightArresting, s.HeightMoving = false, false, false
		s.HeightSliderReady, s.SliderKnown = false, false
		s.CollectiveRemainder = 0
	}
	if !fallback {
		return s.ApplyQ32(in, a, f.Collective, manual, easy)
	}
	copy := in
	copy.Mode = "relative"
	copy.Absolute = false
	if in.Slider {
		s.HeightManualReady = true
		return s.Apply(copy, a, f.Collective, manual)
	}
	if !s.HeightManualReady {
		if in.Axes[2] == 0 && !manual && !in.Digital[2][0] && !in.Digital[2][1] {
			s.HeightManualReady = true
		}
		copy.Bound[2] = false // A held climb gesture cannot become a throttle step.
	}
	return s.Apply(copy, a, f.Collective, manual)
}
