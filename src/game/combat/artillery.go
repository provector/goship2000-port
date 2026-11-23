package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// UpdateArtillery is one ascending source157f row visit, D631..DA98. It
// consumes only the actual launch-owned support prefix and shared AA2 request.
// Death uses the already shared kind19 P2/P3 replacement consumer.
func (s *State) UpdateArtillery(actor int, in Input) error {
	e, err := s.entity(actor)
	if err != nil {
		return err
	}
	support := s.init.Support
	retained := support != nil && s.missionActorsEnabled && support.Configuration.HasRetainedPrefixPolicy()
	if support == nil || !retained && (support.Configuration.Profile != gamestate.SupportInitializationProfile || support.Configuration.Mode != 1) {
		return fmt.Errorf("artillery row%d lacks validated mission-event support prefix", actor)
	}
	admitted := false
	for _, row := range support.Configuration.Rows {
		if row == actor {
			admitted = true
		}
	}
	if retained && support.Configuration.ControllerKind(actor) != 0x19 {
		admitted = false
	}
	if !admitted || actor < 1 || actor >= 80 || e.FlightSlot >= 0 || e.Kind != 0x19 {
		return fmt.Errorf("artillery row%d is outside its validated source span", actor)
	}
	definition, err := s.definition(2)
	if err != nil {
		return err
	}
	if !retained && (definition.RawWSHDID != int(support.Configuration.PrefixRaw[0]) || definition.RawWSHDID < 25 || definition.RawWSHDID > 27) {
		return fmt.Errorf("artillery compact2 does not carry its rawWSHD25..27 prefix")
	}
	switch e.Participation {
	case 0:
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 1
		e.RawMotion = 30
		s.syncDeath(e)
		return nil
	case 2, 3:
		_, err = s.advanceDeathRow(actor, in)
		return err
	case 1:
	default:
		return nil
	}
	if support.Request == 0 || !in.E0 {
		return nil
	}
	dx, dz := delta(support.Target.H0, e.Pose.H0), delta(support.Target.H1, e.Pose.H1)
	// D71A subtracts actual yaw from desired, then NEG and SAR4. Preserve
	// signed16 error before easing and the strict pre-easing absolute bound.
	yawError := int16(e.Pose.Angles[1] - s.angle(-dx, dz))
	e.Pose.Angles[1] -= uint16(yawError >> 4)
	s.syncDeath(e)
	if abs(yawError) >= 0x100 || byte(support.Request)&1 != byte(actor)&1 {
		return nil
	}
	support.Request--
	low := byte(support.Request) & 0x7f
	impact := low < 16
	muzzle := low > 0x60 && low < 0x70
	if !impact && !muzzle {
		return nil
	}
	// D76F/D8FA subtract lowthree request bits, add8 and mask0F. Source
	// AC9 is not advanced here; support does not steal player's next slot.
	slot := int((byte(s.NextSlot) - byte(support.Request&7) + 8) & 15)
	if s.Slots[slot].Lifetime != 0 {
		return nil
	}
	old := s.Slots[slot]
	p := &s.Slots[slot]
	p.Lifetime, p.Flags, p.Speed, p.Parameter, p.Shooter, p.Target, p.Template = 32, 1, 200, 0, byte(actor), 0, 0
	p.Packed = (old.Packed & 3) | 8
	if impact {
		if in.Random == nil {
			return fmt.Errorf("artillery impact requires shared gameplay RNG")
		}
		p.Pose = support.Target
		p.Pose.Height = 2000
		for axis := 0; axis < 2; axis++ {
			jitter := int32(in.Random() & 0xfff)
			if jitter&1 == 0 {
				jitter = -jitter
			}
			if axis == 0 {
				p.Pose.H0 += uint32(jitter)
			} else {
				p.Pose.H1 += uint32(jitter)
			}
		}
		// Anchor+0C is posePITCH (payload+10), not yaw. The source writesC000
		// so these rounds descend; the old compact report's "yaw" was imprecise.
		p.Pose.Angles[0] = 0xc000
		s.move(&p.Pose, int16(((uint16(slot)+in.Phase)&15)<<6))
	} else {
		p.Pose = e.Pose
		p.Pose.Angles[0] = 0x1000
		p.Speed = 50
		s.move(&p.Pose, 32)
		p.Packed = 0 // D9E6 final writer, after constructing the muzzle payload.
	}
	p.AuxiliaryAngles = p.Pose.Angles
	s.emit("support-launch", slot, actor, 0, 0, "157f:D631..DA98")
	return nil
}
