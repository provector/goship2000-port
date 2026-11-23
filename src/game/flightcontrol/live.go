package flightcontrol

// LiveInput is an application-tick sample, independent of device identity or
// presentation. New films carry this optional versioned operand. Historical
// films never acquire it. Values use the original signed +/-127 input precision.
type LiveInput struct {
	Mode     string `json:",omitempty"`
	Slider   bool   `json:",omitempty"`
	Axes     [4]int16
	Bound    [4]bool
	Digital  [4][2]bool
	Lever    uint16
	Absolute bool
	Epoch    uint64
}

func (s LiveInput) Valid() bool {
	for _, v := range s.Axes {
		if v < -127 || v > 127 {
			return false
		}
	}
	return s.Mode == "" || (s.Mode == "absolute" || s.Mode == "relative" || s.Mode == "relative_height") && s.Absolute == (s.Mode == "absolute")
}

// LiveState owns only input adaptation. Flight physics and its widths/timing
// remain in flight.Step. A parked absolute lever never changes restored thrust.
type LiveState struct {
	HeightAxisSampled    bool   `json:",omitempty"`
	HeightAxisAnchor     int16  `json:",omitempty"`
	HeightManualFallback bool   `json:",omitempty"`
	HeightManualReady    bool   `json:",omitempty"`
	HeightSliderReady    bool   `json:",omitempty"`
	SliderAnchor         uint16 `json:",omitempty"`
	SliderKnown          bool   `json:",omitempty"`
	HeightTarget         int16  `json:",omitempty"`
	HeightKnown          bool   `json:",omitempty"`
	HeightArresting      bool   `json:",omitempty"`
	HeightMoving         bool   `json:",omitempty"`
	HeightPhase          uint8  `json:",omitempty"`
	Yaw                  int16
	Epoch                uint64
	Engaged, KnownLever  bool
	PreviousLever        uint16
	PickupAnchor         uint16
	CollectiveRemainder  int32
}

func liveAxis(previous int16, negative, positive bool, analog int16, bound bool) int16 {
	if negative && positive {
		return 0
	}
	if negative || positive {
		return advanceAxis(previous, negative, positive)
	}
	if bound {
		return analog
	}
	return advanceAxis(previous, false, false)
}

// Apply follows the logical source update, not host FPS. Relative collective
// has the same full-deflection rate as one rotor-on keyboard step per source
// update; fractional deflections accumulate rather than losing precision.
func (s *LiveState) Apply(in LiveInput, a *Axes, collective int16, keyboardChange bool) int16 {
	if in.Epoch != s.Epoch {
		*s = LiveState{Epoch: in.Epoch}
	}
	a.Y = liveAxis(a.Y, in.Digital[0][0], in.Digital[0][1], in.Axes[0], in.Bound[0])
	a.X = liveAxis(a.X, in.Digital[1][0], in.Digital[1][1], in.Axes[1], in.Bound[1])
	s.Yaw = liveAxis(s.Yaw, in.Digital[3][0], in.Digital[3][1], in.Axes[3], in.Bound[3])
	lo, hi := in.Digital[2][0], in.Digital[2][1]
	if keyboardChange || lo || hi {
		s.Engaged = false
		s.PickupAnchor = in.Lever
	}
	if in.Bound[2] && in.Absolute {
		value := int32(in.Lever) * 31
		target := int32(collective) * 65535
		previous := int32(s.PreviousLever) * 31
		// About0.8% of full travel: a single raw potentiometer jitter count
		// cannot take collective back from the keyboard. Use an anchor so slow
		// deliberate movement accumulates instead of requiring a sudden jump.
		moved := s.KnownLever && abs32(int32(in.Lever)-int32(s.PickupAnchor)) >= 512
		// One source ordinal of pickup tolerance; movement is mandatory.
		if !keyboardChange && !lo && !hi && moved && (abs32(value-target) <= 65535 || previous <= target && value >= target || previous >= target && value <= target) {
			s.Engaged = true
		}
		if s.Engaged && !keyboardChange && !lo && !hi {
			collective = int16((uint32(in.Lever)*31 + 32767) / 65535)
		}
		if !s.KnownLever {
			s.PickupAnchor = in.Lever
		}
		s.PreviousLever, s.KnownLever = in.Lever, true
		s.CollectiveRemainder = 0
	} else if in.Bound[2] && in.Slider {
		if !s.SliderKnown || keyboardChange || lo || hi {
			s.SliderAnchor = in.Lever
			s.SliderKnown = true
			s.CollectiveRemainder = 0
		} else if delta := int32(in.Lever) - int32(s.SliderAnchor); abs32(delta) >= 256 {
			s.CollectiveRemainder += delta * 31
			s.SliderAnchor = in.Lever
			collective += int16(s.CollectiveRemainder / 65535)
			s.CollectiveRemainder %= 65535
		}
	} else if in.Bound[2] && in.Mode == "relative_height" && !in.Slider {
		// The Easy assist owns this centered axis below, once state is available.
	} else if in.Bound[2] && !lo && !hi && !keyboardChange {
		s.CollectiveRemainder += int32(in.Axes[2])
		collective += int16(s.CollectiveRemainder / 127)
		s.CollectiveRemainder %= 127
	} else {
		s.CollectiveRemainder = 0
	}
	return max(0, min(31, collective))
}
func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
