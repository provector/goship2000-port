package flight

import (
	"fmt"

	"gunship2000-assets/game/world"
)

// DefenseOwnership combines accepted held input with the source's late copilot
// decisions. Array order is radar, IR, as in the existing manual input map.
// RadarToken retains the one token while damaged equipment suppresses its output;
// it is not another jammer. IRCharge is still the only shared charge reservoir.
// Instances are immutable after publication: all writers allocate a replacement
// so copying Defenses by value also isolates historical checkpoint ownership.
type DefenseOwnership struct {
	ManualHeld    [2]bool
	Automatic     [2]bool
	RadarToken    uint16
	StatusPointer uint16 // mainDS406A:3134 = descriptor312C+8, decoy model
}

// EnableR3 opts a new launch into the Step54 manual/automatic ownership policy.
// It is idempotent and must not be applied when restoring an older film.
func (d *Defenses) EnableR3() {
	if d.R3 == nil {
		d.R3 = &DefenseOwnership{StatusPointer: 0x3078}
	}
}

// EffectiveR3 is the source-word publication consumed by seekers and cockpit.
// Newly damaged equipment can suppress those readers immediately while skipped
// 58F2 retains canonical automatic intent and charge/token for the next visit.
// Historical profiles retain their original publication unchanged.
func (d Defenses) EffectiveR3(damage byte) Defenses {
	if d.R3 != nil && damage&0x40 != 0 {
		d.IRActive, d.RadarSignature = 0, 0
	}
	return d
}

// SetR3ManualHeld runs after early counters and before current incoming visits.
// Releasing/focus cleanup removes only physical input. Equipment damage suppresses
// effective output without erasing an already recorded automatic decision.
func (d *Defenses) SetR3ManualHeld(held [2]bool, damage byte, random func() uint16) error {
	if d.R3 == nil {
		return fmt.Errorf("R3 defense ownership is not enabled")
	}
	next := *d
	owner := *d.R3
	owner.ManualHeld = held
	next.R3 = &owner
	if err := next.resolveR3Jammers(damage, random); err != nil {
		return err
	}
	*d = next
	return nil
}

// R3DefenseRequest contains the physical AEB latch, never held manual bits. The
// caller runs the weapon arm first, preserving its earlier RNG and slot writes.
type R3DefenseRequest struct {
	Switches     uint16
	Damage       byte
	E0           bool
	Phase        byte
	Threats      uint16
	Pose         Pose
	DF           uint8
	Angles       *world.Bundle
	SoundEnabled bool // original DE!=0 request gate; host mute remains transport
}

// R3DefenseResult reports this reached 595C..5B62 visit. SoundRequests preserve
// source order (2A flare, then 0A chaff), independently of device admission.
// StatusPointer is zero unless this visit writes mainDS406A:3134. Despite the
// historical field name, this is a decoy render-model pointer, never radio text.
type R3DefenseResult struct {
	ConsumedAEB   bool
	Deployments   []DefenseAction
	SoundRequests []byte
	StatusPointer uint16
}

// ApplyR3Automatic is the late source 595C..5B62 defense arm. It deliberately
// does not advance counters/drift, clear the caller's AEB, or revisit projectiles.
// A failed dependency leaves defenses and AEB unconsumed rather than silently
// accepting a partially implemented positive decision.
func (d *Defenses) ApplyR3Automatic(in R3DefenseRequest, random func() uint16) (R3DefenseResult, error) {
	var out R3DefenseResult
	if d.R3 == nil {
		return out, fmt.Errorf("R3 defense ownership is not enabled")
	}
	if in.Switches&0x08 == 0 || in.Damage&0x40 != 0 || !in.E0 || in.Phase&1 != 0 {
		return out, nil
	}
	// Validate all branch dependencies before spending either store or RNG.
	flare := in.Threats&0x80 != 0 && d.Flares > 0 && d.FlareTimer == 0
	chaff := in.Threats&0x40 != 0 && d.Chaff > 0 && d.ChaffTimer == 0
	if (flare || chaff) && (in.Angles == nil || len(in.Angles.Sine) != 1025) {
		return out, fmt.Errorf("R3 automatic decoy requires source sine table")
	}
	next := *d
	owner := *d.R3
	next.R3 = &owner
	// 5996..5A79: stock>0 with active timer skips both release and fallback.
	// A previous fallback remains until a reached absent-bit branch clears it.
	if in.Threats&0x80 == 0 {
		next.R3.Automatic[1] = false
	} else if d.Flares <= 0 {
		next.R3.Automatic[1] = true
	} else if flare {
		next.Activate(LaunchFlare, 0, in.Pose, in.DF, in.Angles, nil)
		out.Deployments = append(out.Deployments, LaunchFlare)
		out.StatusPointer = 0x3078
		if in.SoundEnabled {
			out.SoundRequests = append(out.SoundRequests, 0x2a)
		}
	}
	// 5A7C..5B5F: radar follows IR and replaces the same decoy/drift/status.
	if in.Threats&0x40 == 0 {
		next.R3.Automatic[0] = false
	} else if d.Chaff <= 0 {
		next.R3.Automatic[0] = true
	} else if chaff {
		next.Activate(LaunchChaff, 0, in.Pose, in.DF, in.Angles, nil)
		out.Deployments = append(out.Deployments, LaunchChaff)
		out.StatusPointer = 0x30ce
		if in.SoundEnabled {
			out.SoundRequests = append(out.SoundRequests, 0x0a)
		}
	}
	if err := next.resolveR3Jammers(in.Damage, random); err != nil {
		return R3DefenseResult{}, err
	}
	*d = next
	out.ConsumedAEB = true // reached the sole 5B62 clear, including absent bits
	return out, nil
}

// resolveR3Jammers merges intent into the existing source words. Ownership
// changes reuse an effective token/remaining charge; no owner has a private copy
// of the IR reservoir. Natural early recharge remains AdvanceCounters' owner.
func (d *Defenses) resolveR3Jammers(damage byte, random func() uint16) error {
	o := d.R3 // caller already made this instance private
	wantsRadar := o.ManualHeld[0] || o.Automatic[0]
	wantsIR := o.ManualHeld[1] || o.Automatic[1]
	if o.RadarToken == 0 && d.RadarSignature != 0 {
		o.RadarToken = d.RadarSignature
	}
	if !wantsRadar {
		o.RadarToken = 0
	}
	if damage&0x40 != 0 {
		if d.IRActive != 0 {
			d.IRCharge = d.IRActive
		}
		d.IRActive, d.RadarSignature = 0, 0
		return nil
	}
	if wantsRadar && o.RadarToken == 0 {
		if random == nil {
			return fmt.Errorf("R3 inactive radar jammer requires source RNG")
		}
		o.RadarToken = (random() & 7) + 1
	}
	d.RadarSignature = o.RadarToken
	if !wantsIR {
		if d.IRActive != 0 {
			d.IRCharge = d.IRActive
		}
		d.IRActive = 0
	} else if d.IRActive == 0 {
		d.IRActive = d.IRCharge
	}
	return nil
}
