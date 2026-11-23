package flight

import "gunship2000-assets/game/world"

// DefenseAction is a semantic operation. Input events and their logical
// scheduling belong to the controller; rendering never calls Activate.
type DefenseAction uint8

const (
	LaunchChaff DefenseAction = iota
	LaunchFlare
	ToggleRadarJammer
	ToggleIRJammer
)

// Defenses retains source AF5/7/9/B and shared35ea/63c..640. RadarSignature
// is a token, not a duration. Counts are cartridges: each source deployment
// releases a group of three (PC manual printed29). This is independent of the
// 1/2/4 rocket-salvo setting. Signed depletion can intentionally produce -1/-2.
type Defenses struct {
	Chaff, Flares          int16
	ChaffTimer, FlareTimer int16
	IRActive, IRCharge     uint16
	RadarSignature         uint16
	Decoy                  Pose
	DriftH0, DriftH1       int16
	DriftHeight            int16
	// R3 is absent in historical checkpoints. Ownership is a new port input
	// policy; the original AF9/AFB words remain the effective seeker inputs.
	R3 *DefenseOwnership `json:"R3,omitempty"`
}

func NewDefenses(chaff, flares int16) Defenses {
	// DATA272d:2d14 is initialized0040; active words start zero.
	return Defenses{Chaff: chaff, Flares: flares, IRCharge: 64}
}

// AdvanceCounters is source02d1..0323, under the early E0 gate, before a
// current-frame activation. Turning IR off retains charge; inactive recharges.
func (d *Defenses) AdvanceCounters(e0 bool) {
	if !e0 {
		return
	}
	if d.FlareTimer != 0 {
		d.FlareTimer--
	}
	if d.ChaffTimer != 0 {
		d.ChaffTimer--
	}
	if d.IRActive != 0 {
		d.IRActive--
		d.IRCharge = d.IRActive
	} else if d.IRCharge < 64 {
		d.IRCharge++
	}
}

// ReconcileJammerRequests is the clearing subset of section1:58f2..5b62.
// It runs after the manual command and player update, before decoy/HUD work.
// CP/G Weapons(18h) and Countermeasures(08h) both enable this later writer:
// absent incoming AEB80/40 requests clear the corresponding manual jammer.
// Request-driven deployments/automation remain owned by the future combat
// mechanism. Passing zero requests is the admitted no-incoming-weapons case.
func (d *Defenses) ReconcileJammerRequests(switches uint16, damage byte, e0 bool, phase byte, requests uint16) {
	if switches&0x08 == 0 || damage&0x40 != 0 || !e0 || phase&1 != 0 {
		return
	}
	if requests&0x80 == 0 {
		d.IRActive = 0
	}
	if requests&0x40 == 0 {
		d.RadarSignature = 0
	}
}

// Activate compiles source0fd0/107a/10dc/1184. The inspected common guard is
// damage5fe.bit40; decoy availability is its signed runtime count. There is no
// invented aircraft-type whitelist. Source trigonometry keeps Q14/word behavior.
func (d *Defenses) Activate(action DefenseAction, damageMask byte, pose Pose, df uint8, angles *world.Bundle, random func() uint16) bool {
	if damageMask&0x40 != 0 {
		return false
	}
	switch action {
	case ToggleIRJammer:
		if d.IRActive != 0 {
			d.IRCharge = d.IRActive
			d.IRActive = 0
		} else {
			d.IRActive = d.IRCharge
		}
		return true
	case ToggleRadarJammer:
		if d.RadarSignature != 0 {
			d.RadarSignature = 0
		} else {
			if random == nil {
				return false
			}
			d.RadarSignature = (random() & 7) + 1
		}
		return true
	case LaunchChaff, LaunchFlare:
		if angles == nil || len(angles.Sine) != 1025 {
			return false
		}
		if action == LaunchChaff {
			if d.Chaff <= 0 {
				return false
			}
			d.Chaff -= 3
			d.ChaffTimer = 10
			d.DriftHeight = -int16(df)
		} else {
			if d.Flares <= 0 {
				return false
			}
			d.Flares -= 3
			d.FlareTimer = 10
			d.DriftHeight = -int16(df) * 2
		}
		d.Decoy = pose // one shared decoy: a new category replaces the same history.
		d.DriftH0 = world.SourceSinAmplitude(pose.Angles[1], int16(df)*12, angles.Sine)
		d.DriftH1 = -world.SourceCosAmplitude(pose.Angles[1], int16(df)*12, angles.Sine)
		if d.R3 != nil {
			owner := *d.R3
			owner.StatusPointer = 0x3078 // mainDS406A:3134, shared decoy model
			if action == LaunchChaff {
				owner.StatusPointer = 0x30ce
			}
			d.R3 = &owner
		}
		return true
	}
	return false
}

// AdvanceDecoy is source1faf..1ffe, after the player model. The source compares
// height with the negative vertical increment, not a generic height>=ground test.
func (d *Defenses) AdvanceDecoy() {
	if d.ChaffTimer|d.FlareTimer == 0 || d.DriftHeight >= d.Decoy.Height {
		return
	}
	d.Decoy.H0 += uint32(int32(d.DriftH0))
	d.Decoy.H1 += uint32(int32(d.DriftH1))
	d.Decoy.Height += d.DriftHeight
}

// GuidanceResponse is the closed seeker-interface portion of family3. It
// deliberately does not create/update a projectile or run an autonomous attack.
// The caller owns source yaw-before-decoy-copy and subsequent vertical guidance.
func (d Defenses) GuidanceResponse(flags uint16, previousRadarSignature uint16) (uint16, bool) {
	if flags&0x8000 != 0 {
		return flags, false
	}
	if flags&0x80 != 0 && d.IRActive != 0 || flags&0x40 != 0 && d.RadarSignature != 0 && d.RadarSignature != previousRadarSignature {
		return flags&3 | 0x20, true
	}
	return flags, false
}
