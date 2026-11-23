package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// resolveFixedContact is source0975:9D1C..9E43. The incoming caller has
// already accepted geometric contact and classified the actual nonplayer
// kind10h bit. This is not NPC7468: it has no defense, bucket or RNG draw.
func (s *State) resolveFixedContact(victim, shooter, slot int, in Input) error {
	e, err := s.entity(victim)
	if err != nil {
		return err
	}
	if e.Participation != 1 { //9D2F, before any table/score/loss access.
		return nil
	}
	// DATA272D:2E62 is four signed words2/4/6/8. ADD and JGE compare
	// the wrapped signed16 accumulator against signed OBJECT+1Dh.
	increment := [4]int16{2, 4, 6, 8}[s.init.Switches&3]
	accumulator := int16(binary.LittleEndian.Uint16(e.DynamicRaw[4:])) + increment
	threshold := int16(binary.LittleEndian.Uint16(e.Object.Raw[29:]))
	destroyed := accumulator >= threshold
	if destroyed && e.Object.Raw[0]&0x80 != 0 && s.init.Targets == nil {
		return fmt.Errorf("fixed victim%d FARP loss lacks original TARGET carrier", victim)
	}
	binary.LittleEndian.PutUint16(e.DynamicRaw[4:], uint16(accumulator))
	s.emit("fixed-damage", slot, shooter, victim, increment, "0975:9D46..9D70 fixed contact; no RNG")
	if !destroyed {
		return nil
	}
	s.sound(in, slot, 0x0f, "0975:9D73..9D81 fixed destruction")
	score := int16(binary.LittleEndian.Uint16(e.Object.Raw[27:]))
	s.Score += score //9D94: signed word addition wraps; no objective-count owner.
	binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
	e.Participation, e.RawMotion = 2, 4 //9DA7/9DAD: next actor visit owns death.
	s.syncDeath(e)
	if e.Object.Raw[0]&0x80 != 0 {
		if s.init.Sortie == nil {
			// Fresh AF1 is source zero. The existing carrier also owns service
			// eligibility and is already part of checkpoints and Resume.
			s.init.Sortie = &gamestate.SortieState{}
		}
		s.init.Sortie.FARPLosses++ //9DC4, including signed16 wrap.
		losses := s.init.Sortie.FARPLosses
		limit := int16(binary.LittleEndian.Uint16(s.init.Targets.Records[1][50:]))
		notice := uint16(0)
		if losses == limit {
			notice = 14
		} else if losses < limit {
			notice = 15
		}
		if notice != 0 {
			s.Events = append(s.Events, Event{Kind: "notification", Actor: 0, Target: 0, Notification: notice, Source: "0975:9DC4..9DFB FARP loss"})
		}
	}
	s.appendResult(Result{Kind: 1, Actor: byte(shooter), Target: byte(victim), Timestamp: in.Phase})
	if victim&1 != 0 {
		s.Credit[victim>>1] = s.Credit[victim>>1]&0x0f | 0x60
	} else {
		// Literal9E3A/F6 mask, not the normal player/wingman kill credit.
		s.Credit[victim>>1] = s.Credit[victim>>1]&0xf6 | 6
	}
	s.Events = append(s.Events, Event{Kind: "destroyed", Slot: slot, Actor: shooter, Target: victim, Value: score, Source: "0975:9D84..9E43 fixed destruction", Pose: e.Pose})
	return nil
}
