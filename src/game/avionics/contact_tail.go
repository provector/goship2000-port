package avionics

import (
	"fmt"
	"gunship2000-assets/game/world"
)

// selectForActor transcribes53d4's nonplayer selector. Unlike the player
// selector, it never changes AB5 or promotes a logistics contact. The initial
// candidate is checked before the search's current-player-target exclusion.
func (s *State) selectForActor(start int, in Input) uint16 {
	i := start & 7
	valid := func(i int) bool {
		c := s.Candidates[i]
		return c.Distance < 0x1000 && c.Angle < 0x2000 && int(c.Row) < EntityCapacity && s.Contacts[c.Row].Flags&7 == 2 && in.Entities[c.Row].Kind&0x70 == 0
	}
	if valid(i) {
		return uint16(s.Candidates[i].Row) + 1
	}
	for n := 0; n < 8; n++ {
		i = (i + 1) & 7
		if valid(i) && uint16(s.Candidates[i].Row)+1 != s.Target {
			return uint16(s.Candidates[i].Row) + 1
		}
	}
	return 0
}

// advanceActorTail is reached only after the existing five shared draws and
// permutation. It consumes no random numbers. Source actors already ran this
// step; their new assignments are consumed on the next actor visit.
func (s *State) advanceActorTail(in Input) error {
	lower := uint16(0)
	following := 0
	set := func(actor int, target int8) error {
		in.Entities[actor].Control[3] = byte(target)
		if in.SetControlTarget == nil {
			return fmt.Errorf("P4 contact tail needs actual CONTROL target owner")
		}
		return in.SetControlTarget(actor, target)
	}
	for ordinal := 1; ordinal < 5; ordinal++ {
		if !in.Entities[ordinal].Present || in.Entities[ordinal].Object[10]&0x20 == 0 {
			break
		}
		actor := int(s.Permutation[ordinal-1])
		if actor < 1 || actor >= 5 || !in.Entities[actor].Present || in.Entities[actor].FlightSlot != actor {
			return fmt.Errorf("contact permutation actor%d lacks actual flight CONTROL", actor)
		}
		e := in.Entities[actor]
		target := int8(e.Control[3])
		if target < 0 {
			target++
			if err := set(actor, target); err != nil {
				return err
			}
			if target < 0 {
				continue
			}
		}
		if e.Control[1] == 0 {
			if err := set(actor, 0); err != nil {
				return err
			}
			continue
		}
		if target > 0 && (int(target) > EntityCapacity || !in.Entities[int(target)-1].Present) {
			return fmt.Errorf("contact actor%d target%d lacks actual source dynamic row", actor, target)
		}
		if target > 0 && in.Entities[int(target)-1].Participation != 1 {
			if err := set(actor, 0); err != nil {
				return err
			}
			continue
		}
		if e.Control[1] == 1 && int8(e.Control[5]) == int8(in.PlayerRow) {
			if following >= len(s.TailDraws)-1 {
				return fmt.Errorf("contact following draw cursor outside source four draws")
			}
			chosen := s.selectForActor(int((s.Cursor+(s.TailDraws[following]&3))&7), in)
			following++
			if err := set(actor, int8(chosen)); err != nil {
				return err
			}
			if chosen != 0 {
				c := &s.Contacts[chosen-1]
				if c.Flags&0xf8 != 0xf8 {
					c.Flags = c.Flags&7 | byte(actor<<3)
				}
			}
			continue
		}
		if in.DisableAutonomy {
			continue
		}
		best := uint16(0x800)
		chosen := target
		limit := Envelope(e.Pose.Height, 0, in.Switches&0x100 != 0, in.Switches&0x200 != 0, false)
		for row, candidate := range in.Entities {
			if !candidate.Present || s.Contacts[row].Flags&7 == 2 || candidate.Kind&0x60 != 0 || candidate.Participation != 1 {
				continue
			}
			metric := Metric(Delta(candidate.Pose.H0, e.Pose.H0), Delta(candidate.Pose.H1, e.Pose.H1))
			if metric >= limit || metric >= best || metric <= lower {
				continue
			}
			best, chosen = metric, int8(row+1)
			if e.Control[1] != 1 {
				lower = 0
			}
		}
		if best == 0x800 {
			continue
		}
		if err := set(actor, chosen); err != nil {
			return err
		}
		row := int(chosen) - 1
		if row < 0 || row >= EntityCapacity {
			continue
		}
		d := Descriptor{}
		if err := s.followerQuery(&d, e.Pose, in.Entities[row], in, byte(actor)); err != nil {
			return err
		}
		if d.Status == 0 {
			if err := set(actor, 0); err != nil {
				return err
			}
			continue
		}
		lower = best
		c := &s.Contacts[row]
		c.Flags = (c.Flags&0xfe|6)&7 | byte(actor<<3)
		c.Kind, c.H0, c.H1 = in.Entities[row].Kind, in.Entities[row].Pose.H0, in.Entities[row].Pose.H1
		// Source does not publish a new yaw in this5118 tail.
		if c.Kind&0x10 != 0 {
			if err := set(actor, 0); err != nil {
				return err
			}
			if in.ContactOrder != nil {
				if err := in.ContactOrder(actor, row); err != nil {
					return err
				}
			} else if c.Kind == 0x1b && e.Control[0] != 0 {
				return fmt.Errorf("source contact retask actor%d target%d needs route owner", actor, row)
			}
			s.ReachedContactOrderTail(c.Kind, in.Entities[row].Object[0])
		}
	}
	return nil
}

func (s *State) followerQuery(d *Descriptor, from Pose, e Entity, in Input, observer byte) error {
	d.Remaining = Budget(in.Switches)
	d.Pose = from
	metric := Metric(Delta(e.Pose.H0, from.H0), Delta(e.Pose.H1, from.H1))
	step := int16(metric) >> 1
	if int16(metric) >= 0x800 {
		step = 0x400
	}
	d.Angles[1] = bearing(from, e.Pose, in.Assets)
	d.Angles[0] = world.SourceAngle((e.Pose.Height-from.Height+32)>>4, int16(metric), in.Assets)
	oldYaw := s.Contacts[e.Row].Yaw
	for d.Remaining != 0 {
		if _, err := s.queryBatch(d, e, step, in, observer); err != nil {
			return err
		}
	}
	s.Contacts[e.Row].Yaw = oldYaw
	return nil
}
