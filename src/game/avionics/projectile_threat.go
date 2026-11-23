package avionics

import "gunship2000-assets/game/world"

const ProjectileThreatSlots = 24

// ProjectileThreatInput preserves source slot domains; shooter/target are not
// contact-row+1 or presentation allegiance. Life0 tails are deliberately excluded.
type ProjectileThreatInput struct {
	Flags                 uint16
	Life, Shooter, Target byte
	Parameter             int16
	H0, H1                uint32
}

// ProjectileControlRelation is admitted only when the adapter has established
// the source70-byte control-row identity. Unknown NPC aliases stay unsupported.
type ProjectileControlRelation struct {
	Known  bool
	Active byte
	Owner  int8
}
type ProjectileThreatInputs struct {
	Player   byte
	Slots    [ProjectileThreatSlots]ProjectileThreatInput
	Controls [EntityCapacity]ProjectileControlRelation
}
type ProjectileThreatPlot struct {
	Slot    byte
	X, Y    int16
	Warning uint16
	Cross   bool
}

// A fixed value snapshot cannot alias mutable projectile-slot storage.
type ProjectileThreatDisplay struct {
	Count        int
	Plots        [ProjectileThreatSlots]ProjectileThreatPlot
	WarningFlags uint16
}
type ThreatFeedback struct {
	Contact    func(ThreatPlot) bool
	Projectile func(ProjectileThreatPlot) bool
}

// ProjectThreats compiles original1975:39b8..3bea. The caller supplies the old
// zoom and ordered logical-pixel feedback, never a physical640 buffer address.
func ProjectThreats(player Pose, assets *world.Bundle, zoom, phase uint16, in ProjectileThreatInputs, feedback func(ProjectileThreatPlot) bool) (out ProjectileThreatDisplay) {
	related := func(row byte) bool {
		if int(row) >= len(in.Controls) {
			return false
		}
		c := in.Controls[row]
		return c.Known && c.Active == 1 && int16(c.Owner) == int16(in.Player)
	}
	for slot, p := range in.Slots {
		if p.Life == 0 || byte(p.Flags)&0xfc == 0 {
			continue
		}
		if p.Target != in.Player && p.Shooter != in.Player && !related(p.Target) && !related(p.Shooter) {
			continue
		}
		// Separate SAR32-by4, low-word subtraction, then SAR16 is significant
		// at native coordinate wrap; shifting the full displacement differs.
		dx, dz := Delta(p.H0, player.H0)>>zoom, Delta(p.H1, player.H1)>>zoom
		if dx < -20 || dx > 20 || dz < -16 || dz > 16 {
			continue
		}
		x := world.SourceSinAmplitude(player.Angles[1], dz, assets.Sine) + world.SourceCosAmplitude(player.Angles[1], dx, assets.Sine)
		y := world.SourceSinAmplitude(player.Angles[1], dx, assets.Sine) - world.SourceCosAmplitude(player.Angles[1], dz, assets.Sine)
		q := ProjectileThreatPlot{Slot: byte(slot), X: x, Y: y}
		if p.Target == in.Player && p.Parameter != 0 && byte(p.Flags)&0x20 == 0 {
			q.Warning = p.Flags & 0xfc
			q.Cross = phase&2 != 0
		}
		out.Plots[out.Count] = q
		out.Count++
		if feedback != nil && feedback(q) {
			out.WarningFlags |= q.Warning
		}
	}
	return
}
