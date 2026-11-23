package combat

import (
	"encoding/binary"
	"fmt"
)

func (s *State) hit(target, shooter int, definition byte, count int16, in Input) error {
	e, err := s.entity(target)
	if err != nil {
		return err
	}
	if e.Participation != 1 {
		return nil
	} //9aad precedes the RNG call.
	// Player subsystem damage and its lifecycle are explicitly not implemented
	// by the stationary9a9c consumer. Family2's zero-row access is still traced.
	if target == s.init.PlayerRow {
		s.emit("unsupported", -1, shooter, target, 0, "family2 row0/player collision requires deferred player-damage contract")
		return nil
	}
	class := int(int8(e.Object.Raw[31]))
	if class > 2 {
		class = 2
	}
	if class < 0 {
		s.emit("unsupported", -1, shooter, target, 0, "negative OBJECT class outside admitted stationary-hit domain")
		return nil
	}
	w, err := s.definition(definition)
	if err != nil {
		return err
	}
	if in.Random == nil {
		return fmt.Errorf("stationary damage requires shared gameplay RNG")
	}
	random := in.Random()
	effect := int16(w.Raw[4+class])
	if random&1 != 0 {
		if int16(random) < 0 {
			effect -= effect >> 2
		} else {
			effect += effect >> 2
		}
	}
	acc := int16(binary.LittleEndian.Uint16(e.DynamicRaw[4:]))
	// One randomized effect is reused for the positive hit count; no extra draw.
	for n := int16(0); n < count; n++ {
		acc += effect
	}
	binary.LittleEndian.PutUint16(e.DynamicRaw[4:], uint16(acc))
	s.emit("damage", -1, shooter, target, effect, "9a9c:9b30")
	threshold := int16(binary.LittleEndian.Uint16(e.Object.Raw[29:]))
	if acc < threshold {
		return nil
	}
	command := uint16(0xf)
	model := e.Object.Raw[12]
	switch model {
	case 0x18, 0x19:
		command = 0x27
	case 0x21, 0xe2:
		command = 0x25
	case 0xd2:
		command = 0x4a
	default:
		if shooter == s.init.PlayerRow || s.init.Entities[shooter].ControlRaw[1] == 1 {
			command = 0x10
		}
	}
	s.sound(in, -1, command, "9a9c:9b63")
	score := int16(binary.LittleEndian.Uint16(e.Object.Raw[27:]))
	s.Score += score
	flags := e.Object.Raw[0]
	if s.Primary.DisplayKind == 3 && flags&0x10 != 0 {
		s.Primary.Destroyed++
		if s.Primary.Destroyed >= s.Primary.Required {
			s.Primary.Achieved = true
			s.Events = append(s.Events, Event{Kind: "notification", Actor: shooter, Target: target, Notification: 0x10, Source: "9a9c:9c32"})
		}
	} else if s.Secondary.DisplayKind == 3 && flags&0x20 != 0 {
		s.Secondary.Destroyed++
		if s.Secondary.Destroyed >= s.Secondary.Required {
			s.Secondary.Achieved = true
			s.Events = append(s.Events, Event{Kind: "notification", Actor: shooter, Target: target, Notification: 0x11, Source: "9a9c:9c65"})
		}
	}
	//9c77 establishes pending destruction, not an immediate model replacement.
	e.Participation = 2
	e.RawMotion = 4
	e.DynamicRaw[1] = 2
	binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
	binary.LittleEndian.PutUint16(e.DynamicRaw[6:], 4)
	s.appendResult(Result{Kind: 1, Actor: byte(shooter), Target: byte(target), Timestamp: in.Phase})
	credit := byte(7)
	if s.init.Entities[shooter].Object.Raw[10]&0x20 != 0 {
		credit = byte(shooter + 1)
	}
	if target&1 == 0 {
		s.Credit[target>>1] = s.Credit[target>>1]&0xf0 | credit
	} else {
		s.Credit[target>>1] = s.Credit[target>>1]&15 | credit<<4
	}
	s.Events = append(s.Events, Event{Kind: "destroyed", Actor: shooter, Target: target, Value: score, Source: "9a9c:9c77", Pose: e.Pose})
	return nil
}

// ApplyStationaryHit exposes the shared mechanism for independent hit fixtures
// and later impact callers; it never admits player/NPC tactical decision logic.
func (s *State) ApplyStationaryHit(target, shooter int, definition byte, count int16, in Input) error {
	if _, err := s.entity(shooter); err != nil {
		return err
	}
	return s.hit(target, shooter, definition, count, in)
}
