package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// ContactOrder consumes only source3ef2's already successful independent-body
// contact tail. The preceding owner has published state6 and cleared CONTROL+3.
// It never runs a second LOS query, draws randomness, or selects an attack.
func (s *State) ContactOrder(actor, target int, currentNotice byte) error {
	if !s.logisticsEnabled {
		return nil
	}
	if actor < 0 || actor >= 5 || actor >= len(s.init.Entities) || target < 0 || target >= len(s.init.Entities) {
		return fmt.Errorf("contact-order actual actor/target is unavailable")
	}
	e, t := &s.init.Entities[actor], &s.init.Entities[target]
	if e.ControlCarrierOnly {
		return fmt.Errorf("contact-order actor%d lacks actual CONTROL", actor)
	}
	if t.Kind == 0x1b && e.ControlRaw[0] != 0 {
		index := int(int8(e.ControlRaw[0]))
		point := 10 * index
		next := point + 30
		if index < 1 || point+8 > len(e.ControlRaw) || next+8 > len(e.ControlRaw) {
			return fmt.Errorf("contact-order actor%d route index%d reaches unsupported CONTROL span", actor, index)
		}
		r := [62]byte{}
		copy(r[34:42], e.ControlRaw[point:point+8])
		if gamestate.ObjectiveDistance(r, t.Pose) < 1024 {
			x, z := t.Pose.H0, t.Pose.H1
			if int32(e.Pose.H0) < int32(x) {
				x -= 128
			} else {
				x += 128
			}
			if int32(e.Pose.H1) < int32(z) {
				z -= 128
			} else {
				z += 128
			}
			binary.LittleEndian.PutUint32(e.ControlRaw[next:], x)
			binary.LittleEndian.PutUint32(e.ControlRaw[next+4:], z)
			s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 18, Source: "535C:survivor route adjustment"})
		}
	}
	if t.Kind == 0x15 && t.Object.Raw[0]&0x30 != 0 && currentNotice != 11 {
		s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 11, Source: "53C7:objective contact"})
	}
	return nil
}
