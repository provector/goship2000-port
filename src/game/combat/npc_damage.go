package combat

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// SetWingmanDamageEnabled is selected before a new R2.2 launch checkpoint.
// Historical films keep94C8's old guard, irrespective of present preferences.
// This policy does not depend on either AI switch or the player's CP/G bits.
func (s *State) SetWingmanDamageEnabled(enabled bool) {
	s.wingmanDamageEnabled = enabled
}

// resolveNonplayerContact is94C8..95FD after geometric contact acceptance.
// The projectile is already snapped/expired. Per-victim defense and7468 remain
// synchronous so later ascending slots see stores, damage, replacement and RNG.
func (s *State) resolveNonplayerContact(slot int, in Input) error {
	p := &s.Slots[slot]
	victim := int(p.Target)
	e := &s.init.Entities[victim]
	//94D4's kind10-bit owner is distinct from ordinary aircraft. Its defined
	//9D2F negative leaf is needed when an earlier shot just produced kind1C/P0.
	if e.Kind&0x10 != 0 {
		if e.Participation != 1 {
			p.Packed = 2 //94EE, with no defense/damage/radar-second/event call.
			return nil
		}
		if s.enemyBehaviorEnabled {
			if err := s.resolveFixedContact(victim, int(p.Shooter), slot, in); err != nil {
				return err
			}
			p.Packed = 2 //94EE follows9D1C, including its below-threshold return.
			return nil
		}
		return fmt.Errorf("accepted hostile impact on actor%d requires active fixed/FARP consumer9D1C", victim)
	}
	stores := s.init.Stores
	if victim < 1 || victim > 4 || e.Kind < 0x20 || e.Kind > 0x26 || stores == nil || stores.NPCDamage == nil || victim >= len(stores.Ships) || victim >= len(stores.Damage) || victim >= len(stores.NPCDamage.Masks) {
		return fmt.Errorf("accepted hostile impact on actor%d lacks R2.2 flight-victim SHIP/damage provenance", victim)
	}
	ship := &stores.Ships[victim]
	if stores.Damage12(victim)[0]&0x40 == 0 {
		//951B radar first,9560 IR second. Stores are signed and SUB3 wraps;
		//the strict comparison is random>own skill.8000 never bypasses this arm.
		for _, defense := range []struct {
			flag  uint16
			store *int16
		}{{0x40, &ship.Chaff}, {0x80, &ship.Flares}} {
			if p.Flags&defense.flag == 0 || *defense.store <= 0 {
				continue
			}
			if ship.Proficiency == nil {
				return fmt.Errorf("accepted hostile impact on actor%d lacks own SHIP proficiency", victim)
			}
			before := *defense.store
			*defense.store -= 3
			draw := in.Random()
			if byte(draw) > *ship.Proficiency {
				p.Packed = 0 //9552: no damage, notice, event, decoy, timer or cue.
				s.incomingOutcome("hostile-defended", slot, int16(byte(draw)), fmt.Sprintf("0975:953A/9584 actor%d defense%02x stores%d->%d rng%04x byte%d > own proficiency%d", victim, defense.flag, before, *defense.store, draw, byte(draw), *ship.Proficiency))
				return nil
			}
		}
	}
	p.Packed = 2 //959C before either synchronous7468 invocation.
	first, err := s.applyNPCDamage(victim, slot, in)
	if err != nil {
		return err
	}
	packed := first
	//95BE examines the original projectile flag even if the first7468 call
	//returned0 or wrecked this actor. The later call retains its own guards.
	if p.Flags&0x40 != 0 && byte(in.Random()) < 200 {
		second, err := s.applyNPCDamage(victim, slot, in)
		if err != nil {
			return err
		}
		packed |= second << 4
	}
	return s.recordAcceptedDamage(victim, packed, in.Phase)
}

func (s *State) applyNPCDamage(victim, slot int, in Input) (uint16, error) {
	result, err := s.init.ApplyAcceptedNPCHit(gamestate.NPCHitContext{VictimRow: victim, Random: in.Random})
	if err != nil {
		return 0, err
	}
	if result.Limit != "" {
		return 0, fmt.Errorf("wingman actor%d damage successor: %s", victim, result.Limit)
	}
	if result.Applied {
		s.init.FollowerFailureKnown[victim] = true
		if systems := in.Incoming.Systems; systems != nil && victim < len(systems.Fuel) {
			systems.Fuel[victim] = s.init.Stores.Ships[victim].Fuel
		}
	}
	if result.DirectWreck {
		e := &s.init.Entities[victim]
		e.DynamicRaw[2], e.ControlRaw[1] = 0, 0 //7A97,7AE0.
		if err := s.replaceDestroyed(victim, true); err != nil {
			return 0, err
		}
	}
	if result.NotifyDamage {
		s.Events = append(s.Events, Event{Kind: "notification", Slot: slot, Actor: victim, Notification: 4, Source: "7468:7B5B NPC damage"})
	}
	if result.Applied {
		s.incomingOutcome("npc-damage", slot, int16(result.ReturnCode), fmt.Sprintf("0975:7468/7A3C victim%d bucket%d damage%v direct_wreck%t", victim, result.Bucket, s.init.Stores.Damage12(victim), result.DirectWreck))
	}
	return result.ReturnCode, nil
}
