package combat

import (
	"fmt"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

// InitializeFreshSeeker selects the proved CRT zero writer only at a fresh
// R5 launch. Checkpoint restoration must restore its recorded shared owner.
func (s *State) InitializeFreshSeeker() {
	s.conditionalSeekerReads = true
	s.incomingAimPose = gamestate.InitializedPose{}
	s.incomingAimKnown = true
}

// IncomingContext is the recovered family3/player consumer boundary. ApplyPlayerHit
// must call real7468 synchronously at this slot visit (once per invocation), not
// queue damage after the24-slot loop. R2.1's natural stationary constructor uses
// this same boundary; diagnostic SupplyIncoming remains explicitly separate.
type IncomingContext struct {
	ApplyPlayerHit   func(slot int) (uint16, error)
	SpeechEnabled    func() bool // Real DE80 may change during an earlier slot's7468.
	Defenses         *flight.Defenses
	Systems          *flight.Systems // R2.2 mirrors actual NPC fuel into the existing burn owner.
	LimitCompression func()
}

// incomingOutcome is bounded operational evidence at an actual terminal
// projectile branch. It is outside FilmState, RNG and player message dispatch.
// Historical interpretations retain their prior event stream.
func (s *State) incomingOutcome(kind string, slot int, value int16, source string) {
	if s.stationaryEnemy == nil {
		return
	}
	p := &s.Slots[slot]
	s.Events = append(s.Events, Event{Kind: kind, Slot: slot, Actor: int(p.Shooter), Target: int(p.Target), Value: value, Source: source, Pose: p.Pose})
}

// SupplyIncoming accepts explicitly supplied source fields, not an enemy AI order.
// Packed is the active rejection threshold, Parameter is gain; the original
// gun/guided constructor gives ENEMY byte1 only ONE of these tagged meanings.
func (s *State) SupplyIncoming(slot int, p Projectile, warning bool) error {
	if slot < 20 || slot >= SlotCount || p.Flags&3 != 3 || p.Target != byte(s.init.PlayerRow) || s.init.PlayerRow != 0 {
		return fmt.Errorf("supplied incoming requires hostile20..23 and direct player0")
	}
	player, err := s.entity(s.init.PlayerRow)
	if err != nil {
		return err
	}
	if s.init.Sortie != nil && s.init.Sortie.Result != nil || s.init.PlayerFlight != nil && s.init.PlayerFlight.Terminal != nil {
		return fmt.Errorf("supplied incoming cannot enter a terminated sortie")
	}
	if player.Participation != 0 || player.Kind != 0xff || p.Lifetime == 0 || p.Speed <= 0 || p.Parameter < 0 || p.Parameter > 255 {
		return fmt.Errorf("incoming retained player/payload outside fresh positive-speed domain")
	}
	if _, err := s.entity(int(p.Shooter)); err != nil {
		return err
	}
	if int(p.Shooter) == s.init.PlayerRow || s.Slots[slot].Lifetime != 0 || s.Slots[slot].Packed != 0 {
		return fmt.Errorf("incoming owner or occupied hostile slot is unavailable")
	}
	s.Slots[slot], s.incomingAdmitted[slot], s.incomingWarning[slot] = p, true, warning
	s.emit("launch", slot, int(p.Shooter), int(p.Target), 0, "supplied family3 source boundary; no autonomous launch")
	return nil
}

func (s *State) stepIncoming(slot int, old byte, in Input) error {
	p, c := &s.Slots[slot], in.Incoming
	voiceEnabled := func() bool {
		if c.SpeechEnabled != nil {
			return c.SpeechEnabled()
		}
		return in.SpeechEnabled
	}
	if c.ApplyPlayerHit == nil || c.Defenses == nil || in.Random == nil {
		return fmt.Errorf("incoming needs synchronous damage, authoritative defenses and shared RNG")
	}
	if s.init.PlayerRow != 0 || int(p.Target) >= len(s.init.Entities) || p.Target == byte(s.init.PlayerRow) && s.init.Entities[0].Participation != 0 {
		return fmt.Errorf("incoming target/retained alias is outside admitted source domain")
	}
	previous := p.Pose
	distance := p.Speed // Already DF-scaled by the original constructor.
	if old >= 131 {
		distance = 32
	}
	s.move(&p.Pose, distance)
	target := &s.init.Entities[p.Target]
	dx, dz := delta(target.Pose.H0, p.Pose.H0), delta(target.Pose.H1, p.Pose.H1)
	d := metric(dx, dz)
	yaw := s.angle(-dx, dz) // Original target yaw precedes the decoy pose copy.
	if d < uint16(p.Speed>>4) && uint16(abs(int16(p.Pose.Height-target.Pose.Height))) < 64 {
		p.Pose.H0, p.Pose.H1, p.Pose.Height = target.Pose.H0, target.Pose.H1, target.Pose.Height
		p.Lifetime = 0 // Snap/expire before the acceptance draw, including rejection.
		threshold := p.Packed
		acceptance := byte(in.Random())
		if acceptance <= threshold {
			s.incomingOutcome("hostile-miss", slot, int16(acceptance), fmt.Sprintf("0975:9492/9600 acceptance byte%d <= threshold%d", acceptance, threshold))
			p.Packed = 0
			return nil
		}
		s.incomingOutcome("hostile-impact", slot, int16(acceptance), fmt.Sprintf("0975:9492/949C accepted contact byte%d > threshold%d; damage consumer follows", acceptance, threshold))
		if p.Target != byte(s.init.PlayerRow) {
			if s.wingmanDamageEnabled {
				return s.resolveNonplayerContact(slot, in)
			}
			// The real projectile can travel or miss a nonplayer. Its accepted
			// hit reaches94C8..95FD (NPC defense/SHIP damage), a separate owner.
			// Preserve that reached mechanical limit instead of redirecting the
			// shot, applying player damage, or silently awarding a fake miss.
			return fmt.Errorf("accepted hostile impact on actor%d requires original NPC defense/damage consumer94C8..95FD", p.Target)
		}
		// Player precedence is before kind&10 (the player's kindFF has that bit).
		s.emit("shake", slot, int(p.Shooter), int(p.Target), 16, "94a9 player ACD")
		if voiceEnabled() {
			s.sound(in, slot, 0x3b, "94ba player impact voice")
		}
		p.Packed = 2
		first, err := c.ApplyPlayerHit(slot)
		if err != nil {
			return err
		}
		packed := first
		if p.Flags&0x40 != 0 && byte(in.Random()) < 200 {
			second, err := c.ApplyPlayerHit(slot)
			if err != nil {
				return err
			}
			packed |= second << 4
		}
		return s.RecordAcceptedPlayerHit(int(p.Target), packed, in.Phase)
	}
	if d < 0x6000 {
		if p.Target == byte(s.init.PlayerRow) && d < 0x200 {
			s.IncomingThreat |= p.Flags & 0xfc
		}
		chosen := target.Pose
		disable := func(cause string) {
			previousFlags, previousGain := p.Flags, p.Parameter
			p.Flags, p.Parameter = p.Flags&3|0x20, 0
			if s.persistentPlayerThreat && (previousFlags != p.Flags || previousGain != p.Parameter) {
				// Diagnostic only: this proves the seeker lost guidance. A live
				// ballistic round can still contact later; do not report a miss.
				s.incomingOutcome("hostile-guidance-defeated", slot, 0, cause)
			}
		}
		// Source tests active shooter P1; non-IR loses guidance otherwise.
		if s.init.Entities[p.Shooter].Participation != 1 && p.Flags&0x80 == 0 {
			disable("0975:9670..967A inactive shooter disabled non-IR guidance")
			// 967A jumps over every35E0 copy. Gain0 does not suppress the
			// later pitch read. Retain the last established shared scratch;
			// an unknown first-use retained value is not a zero/target default.
			if !s.incomingAimKnown && !s.conditionalSeekerReads {
				return fmt.Errorf("incoming inactive-shooter scratch35E0 provenance unknown")
			}
			chosen = s.incomingAimPose
		} else if p.Target == byte(s.init.PlayerRow) || target.ControlRaw[1] == 1 && int(int8(target.ControlRaw[5])) == s.init.PlayerRow {
			// 967E..96AB: player and a following-player actor share the
			// player's decoy/jammer observations. Independent NPCs use their
			// own live body pose at97B2, without borrowing player defenses.
			def := *c.Defenses
			if c.Systems != nil {
				// R3's accepted ownership policy suppresses effective equipment
				// immediately after an earlier same-pass damage application. The
				// automatic intent/one charge/token remain their canonical owners.
				def = def.EffectiveR3(c.Systems.DamageMasks[0])
			}
			if p.Flags&0x80 != 0 && def.FlareTimer != 0 || p.Flags&0x40 != 0 && def.ChaffTimer != 0 {
				chosen.H0, chosen.H1, chosen.Height = def.Decoy.H0, def.Decoy.H1, uint16(def.Decoy.Height)
			}
			s.incomingAimPose, s.incomingAimKnown = chosen, true
			// Bypass8000 does not bypass the earlier decoy copy.
			if in.TraceIR != nil && p.Flags&0x80 != 0 && int(p.Target) == s.init.PlayerRow {
				in.TraceIR(slot, p.Target, p.Flags, def.IRActive)
			}
			if p.Flags&0x8000 == 0 {
				if p.Flags&0x80 != 0 && def.IRActive != 0 {
					disable("0975:972C..974C effective IR jammer disabled guidance")
				}
				if p.Flags&0x40 != 0 && def.RadarSignature != 0 && def.RadarSignature != s.IncomingRadarHistory {
					s.IncomingRadarHistory = (in.Random() & 7) + 1
					disable("0975:975B..97A8 changed radar token disabled guidance")
				}
			}
		} else {
			s.incomingAimPose, s.incomingAimKnown = chosen, true
		}
		errorAngle := int16(p.Pose.Angles[1] - yaw)
		if uint16(abs(errorAngle)) > 0x4000 {
			p.Lifetime = 0
			s.incomingOutcome("hostile-miss", slot, errorAngle, fmt.Sprintf("0975:97E4/97E9 bearing error%04x outside forward half-plane", uint16(errorAngle)))
			return nil
		}
		if uint16(abs(errorAngle)) >= 0x800 {
			p.Flags &= 3 // Unlike jammer disable, gain is retained.
		} else {
			if !s.incomingAimKnown {
				return fmt.Errorf("incoming pitch read9847 requires known shared35E0 height")
			}
			if c.LimitCompression != nil {
				c.LimitCompression()
			}
			p.Pose.Angles[1] -= uint16(int16((int32(errorAngle) * int32(p.Parameter)) >> 8))
			pitch := int16(s.angle(int16(chosen.Height-p.Pose.Height)>>4, int16(d)))
			p.Pose.Angles[0] = uint16((int16(p.Pose.Angles[0]) >> 1) + (pitch >> 1))
			if p.Target == byte(s.init.PlayerRow) && s.incomingWarning[slot] && p.Parameter != 0 {
				s.incomingWarning[slot] = false // Consume before speech gate, once per slot.
				if voiceEnabled() {
					rel := uint16(target.Pose.Angles[1] - p.Pose.Angles[1] + 0x2000)
					commands := [4]uint16{0x37, 0x35, 0x38, 0x36}
					s.sound(in, slot, commands[rel>>14], "98ce quadrant voice")
				}
			}
		}
	}
	h, err := s.terrain(p, previous)
	if err != nil {
		return err
	}
	if int16(p.Pose.Height) < h {
		p.Lifetime = 0
		p.Packed = 1
		s.incomingOutcome("hostile-miss", slot, h, fmt.Sprintf("0975:9933/993D projectile height%d below terrain%d", int16(p.Pose.Height), h))
	} else if old == 1 && p.Lifetime == 0 {
		s.incomingOutcome("hostile-miss", slot, 0, "0975:86E6 source lifetime decrement reached zero after final movement")
	}
	return nil
}
