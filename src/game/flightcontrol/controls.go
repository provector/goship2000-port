// Package flightcontrol contains the original keyboard-control producers.
// Its update is called by a logical owner, never by a renderer; it does not
// establish the original simulation clock or integrate an aircraft.
package flightcontrol

type Held struct{ Left, Right, Up, Down bool }

// Axes are source DS1332/1334 signed control ordinals, not angles or velocities.
type Axes struct{ X, Y int16 }

func advanceAxis(v int16, negative, positive bool) int16 {
	if !negative && !positive {
		next := v >> 1
		if next == v { // SAR(-1,1) remains -1; source explicitly clears it.
			return 0
		}
		return next
	}
	// Source applies the negative key first, clamps it, then the positive key.
	// Opposite keys at a saturated edge therefore need not cancel.
	if negative {
		v -= 16
		if v < -127 {
			v = -127
		}
	}
	if positive {
		v += 16
		if v > 127 {
			v = 127
		}
	}
	return v
}

func (a *Axes) Advance(h Held) {
	a.X = advanceAxis(a.X, h.Left, h.Right)
	a.Y = advanceAxis(a.Y, h.Up, h.Down)
}

func (a *Axes) Clear() { *a = Axes{} }

type Action uint8

const (
	None Action = iota
	CollectiveUp
	CollectiveDown
	CollectiveUpFast
	CollectiveDownFast
	RotorToggle
	HoverToggle
	LowLimitDown
	LowLimitUp
	PrimaryMFDNext
	SecondaryMFDNext
	Pause
)

// Resolve translates semantic key names. Platform key codes, repetition and
// modal input ownership belong to the caller. No deferred weapon/GPS key is
// captured as an unrelated convenience shortcut.
func Resolve(key string, shift, alt bool) Action {
	if alt {
		if key == "p" {
			return Pause
		}
		return None
	}
	if shift && key != "=" && key != "-" {
		return None
	}
	switch key {
	case "=":
		if shift {
			return CollectiveUpFast
		}
		return CollectiveUp
	case "-":
		if shift {
			return CollectiveDownFast
		}
		return CollectiveDown
	case "9":
		return RotorToggle
	case "0":
		return HoverToggle
	case "c":
		return LowLimitDown
	case "v":
		return LowLimitUp
	case "z":
		return PrimaryMFDNext
	case "x":
		return SecondaryMFDNext
	}
	return None
}

// State names the direct source control words. Natural initialization must come
// from launch/import writers, not this struct's Go zero values. SpeedE2 is kept
// raw until the motion producer supplies its native scale.
type State struct {
	Collective, Rotor, Hover, LowLimit int16
	PrimaryMFD, SecondaryMFD           int16
	SpeedE2                            int16
	AvionicsDamage                     byte
	SecondaryMFDInstalled              bool
	SpeedHold2D06, FlightStage2D08     int16
}

func absWord(x int16) int16 {
	if x < 0 {
		return -x
	}
	return x
}

// Apply performs the original direct action writer. It does not claim that a
// label change implements the downstream hover/rotor physics or MFD recipes.
func (s *State) Apply(a Action) bool {
	before := *s
	switch a {
	case CollectiveUp, CollectiveDown, CollectiveUpFast, CollectiveDownFast:
		delta := int16(4)
		if (a == CollectiveUp || a == CollectiveDown) && s.Rotor != 0 {
			delta = 1
		}
		if a == CollectiveDown || a == CollectiveDownFast {
			delta = -delta
		}
		s.Collective += delta
		if s.Collective < 0 {
			s.Collective = 0
		}
		if s.Collective > 31 {
			s.Collective = 31
		}
	case RotorToggle:
		s.Rotor ^= 1
		if s.Rotor != 0 {
			s.FlightStage2D08 = 4
		} else {
			s.Collective = 0
		}
	case HoverToggle:
		if s.AvionicsDamage&3 == 2 {
			break
		}
		if s.Hover != 0 || s.Rotor == 0 {
			s.Hover = 0
			break
		}
		s.Hover = 2
		if absWord(s.SpeedE2) < 64 {
			s.Hover = 1
		}
		s.SpeedHold2D06 = 0
	case LowLimitDown:
		if s.LowLimit > 64 {
			s.LowLimit -= 64
		}
	case LowLimitUp:
		if s.LowLimit < 320 {
			s.LowLimit += 64
		}
	case PrimaryMFDNext:
		s.PrimaryMFD++
		if s.PrimaryMFD > 5 {
			s.PrimaryMFD = 0
		}
	case SecondaryMFDNext:
		if s.SecondaryMFDInstalled {
			s.SecondaryMFD++
			if s.SecondaryMFD > 5 {
				s.SecondaryMFD = -1
			}
		}
	}
	return before != *s
}
