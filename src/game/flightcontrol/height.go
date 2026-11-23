package flightcontrol

import "gunship2000-assets/game/flight"

// ApplyHeight is the optional native Easy-flight input assist. It adjusts only
// collective, never pose/velocity/terrain/rotor state. The right HUD scale is
// RadarAltitude (publishFlight -> HUDScale), not the separate world-height gauge.
// Neutral captures its source contact-relative height, independent of cameras.
func (s *LiveState) ApplyHeightV7(in LiveInput, f *flight.State, easy, manual bool) {
	if in.Mode != "relative_height" || in.Slider || !in.Bound[2] || !easy {
		s.HeightKnown = false
		// Realistic and slider use ordinary manual relative collective.
		if in.Mode == "relative_height" && !in.Slider && in.Bound[2] && !easy && !manual {
			s.CollectiveRemainder += int32(in.Axes[2])
			f.Collective = max(0, min(31, f.Collective+int16(s.CollectiveRemainder/127)))
			s.CollectiveRemainder %= 127
		}
		return
	}
	if manual || in.Digital[2][0] || in.Digital[2][1] || f.HeightControl != 0 || f.Rotor == 0 || f.ValidContact && in.Axes[2] <= 0 {
		s.HeightKnown = false
		return
	}
	if !s.HeightKnown {
		s.HeightTarget = f.RadarAltitude
		s.HeightKnown = true
		s.HeightMoving = false
		s.HeightPhase = 0
	}
	axis := in.Axes[2]
	want := int16(0)
	if axis != 0 {
		want = int16(int32(axis) * 8 / 127)
		s.HeightTarget = f.RadarAltitude
		s.HeightMoving = true
	} else {
		if s.HeightMoving {
			s.HeightTarget = f.RadarAltitude
			s.HeightMoving = false
		}
		want = max(-6, min(6, (s.HeightTarget-f.RadarAltitude)/6))
	}
	// Existing dynamics change Vertical by at most2 each source iteration.
	// Modest collective increments and a velocity dead band avoid rapid pumping.
	s.HeightPhase = (s.HeightPhase + 1) % 2
	if s.HeightPhase == 0 {
		if f.Vertical < want-1 {
			f.Collective = min(31, f.Collective+1)
		}
		if f.Vertical > want+1 {
			f.Collective = max(0, f.Collective-1)
		}
	}
}
