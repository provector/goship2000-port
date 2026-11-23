package flight

// Systems owns the noncombat fuel countdown and source instrument easing.
// Source fuel is a percentage here; preflight fuel in pounds is not decremented.
type Systems struct {
	Fuel          []int16
	Active        []bool
	Countdown     int16
	Engines       [2]int16
	EngineTargets [2]int16
	Damage        [10]byte
	DamageMasks   [2]byte // DATA5fe/5ff, separate from scalar damage600..609.
	// Original follower oldempty enters RNG/609/descent successors not yet
	// integrated. This native witness excludes their later result admission;
	// it is not a fabricated damage value or source status flag.
	UnsupportedFollowerEmptyBurn byte
}

// FuelBurn publishes the source branch reached before any decrement. Audio
// must not infer exhaustion from the resulting fuel: old1->0 is still the
// positive-fuel branch, and only the next oldempty pass writes609.
type FuelBurn struct {
	PlayerActive bool
	PreviousFuel int16
}

// FuelActor is read at the actual ascending burn visit. Prefix and Control
// remain distinct; Proficiency is SHIP+1d, Height is absolute dynamic height.
type FuelActor struct {
	Prefix, Known bool
	Control       byte
	Height        int16
	Damage603     byte
	Proficiency   byte
}
type FollowerFuelContext struct {
	Read   func(int) FuelActor
	Random func() uint16
	Set609 func(int, byte)
	// Notify is the source0350/0366 nonplayer old-fuel message producer.
	// Interpretation selection is supplied by the caller, never inferred here.
	Notify func(actor int, kind uint16)
}

func NewSystems(fuel []int16, active []bool) Systems {
	return Systems{Fuel: append([]int16(nil), fuel...), Active: append([]bool(nil), active...), Countdown: 150, EngineTargets: [2]int16{15, 15}}
}

// Advance precedes the player model. Original mainloop0323..0510 tests the
// old countdown for zero, then resets to150-collective; there is no rotor guard.
func (s *Systems) Advance(e0 bool, collective int16) FuelBurn {
	return s.AdvanceWithFollowers(e0, collective, nil)
}

func (s *Systems) AdvanceWithFollowers(e0 bool, collective int16, followers *FollowerFuelContext) FuelBurn {
	var burn FuelBurn
	if !e0 {
		return FuelBurn{}
	}
	previous := s.Countdown
	s.Countdown--
	if previous == 0 {
		s.Countdown = 150 - collective
		for i := range s.Fuel {
			if i >= 5 { // Source048f stops at the five ordinary SHIP actor rows.
				break
			}
			var actor FuelActor
			if followers != nil && followers.Read != nil {
				actor = followers.Read(i)
				if !actor.Prefix {
					break
				}
				if actor.Control == 0 {
					continue
				}
			} else if i >= len(s.Active) || !s.Active[i] {
				continue
			}
			oldFuel := s.Fuel[i]
			if oldFuel > 0 {
				if i != 0 && followers != nil && followers.Notify != nil {
					switch oldFuel {
					case 24:
						followers.Notify(i, 5)
					case 10:
						followers.Notify(i, 6)
					}
				}
				s.Fuel[i]--
				// Source039d tests physical603's low two bits. Native supplied
				// damage is player-owned; followers have no accepted-hit producer.
				damage := s.Damage[3]
				if i != 0 {
					damage = actor.Damage603
				}
				if damage&3 == 1 && s.Fuel[i] != 0 {
					s.Fuel[i]--
				}
			} else if i == 0 {
				s.Damage[9] = 1
			} else {
				if followers == nil || !actor.Known || followers.Set609 == nil || (actor.Height > 2 && followers.Random == nil) {
					s.UnsupportedFollowerEmptyBurn |= 1 << uint(i)
				} else {
					damage := byte(2)
					if actor.Height > 2 && uint16(actor.Proficiency) >= followers.Random()&127 {
						damage = 1
					}
					followers.Set609(i, damage)
				}
			}
			if i == 0 {
				// Followers still consume their independently initialized fuel.
				burn = FuelBurn{PlayerActive: true, PreviousFuel: oldFuel}
			}
		}
	}
	return burn
}

// EaseEngines is the visible primary-instrument operation at0786..07f6.
// Its logical owner preserves the source visibility/damage/second-engine gates.
func (s *Systems) EaseEngines(e0, visible, second bool) {
	if !e0 || !visible {
		return
	}
	for i := range s.Engines {
		if i == 1 && !second {
			continue
		}
		if s.Engines[i] < s.EngineTargets[i] {
			s.Engines[i]++
		} else if s.Engines[i] > s.EngineTargets[i] {
			s.Engines[i]--
		}
	}
}
