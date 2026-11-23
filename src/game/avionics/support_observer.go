package avionics

import (
	"fmt"
	"gunship2000-assets/game/world"
)

// ObserveSupport is CD26..CDAD's body-only source0616 transaction. Its fixed64
// samples and source state6 attribution are separate from player/mast sensing.
func (s *State) ObserveSupport(in Input, actor, target int) (bool, error) {
	if actor < 0 || actor >= EntityCapacity || target < 0 || target >= EntityCapacity || !in.Entities[actor].Present || !in.Entities[target].Present || in.Assets == nil {
		return false, fmt.Errorf("Support observer lacks actual source actors")
	}
	from, e := in.Entities[actor].Pose, in.Entities[target]
	dx, dz := Delta(e.Pose.H0, from.H0), Delta(e.Pose.H1, from.H1)
	d := Metric(dx, dz)
	step := int16(d >> 1)
	if d >= 0x800 {
		step = 0x400
	}
	q := Descriptor{Remaining: 64, Pose: from}
	q.Angles[1] = world.SourceAngle(-dx, dz, in.Assets)
	q.Angles[0] = world.SourceAngle((e.Pose.Height-from.Height+32)>>4, int16(d), in.Assets)
	for q.Remaining != 0 {
		if _, err := s.queryBatch(&q, e, step, in, byte(actor)); err != nil {
			return false, err
		}
	}
	if q.Status == 0 {
		return false, nil
	}
	c := &s.Contacts[target]
	c.Flags = byte(actor<<3) | 6
	return true, nil
}
