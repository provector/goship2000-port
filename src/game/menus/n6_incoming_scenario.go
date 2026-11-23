package menus

import (
	"fmt"
	"gunship2000-assets/game/combat"
)

// configureN6IncomingScenario supplies only a created family3 payload. Genuine
// preparation, player damage, source steering/contact and final facts remain live.
// This does not implement or claim an original enemy launch/tactical decision.
func (c *FrontEnd) configureN6IncomingScenario() error {
	p, meta := c.Rendering, c.DevelopmentScenario
	if p == nil || p.Flight == nil || meta == nil || len(p.Initialized.Entities) < 2 {
		return fmt.Errorf("incoming scenario lacks genuine prepared player/owner rows")
	}
	r := p.Flight
	// Replace the exhaustion-specific fuel precondition, not accepted equipment,
	// damage, target, objective, contact, terminal status or resulting career facts.
	r.Systems.Fuel[0], r.Systems.Countdown = 100, 150
	p.Initialized.RandomState = 0x280
	p.Scene.Environment.RNG = 0x280
	meta.InitialRNG = 0x280
	shot := combat.Projectile{Flags: 0x43, Lifetime: 30, Target: 0, Shooter: 1, Speed: 306, Parameter: 64, Packed: 0, Pose: p.Initialized.Entities[0].Pose}
	shot.Pose.H1 += 4096
	shot.Pose.Angles = [3]uint16{0, 0x8000, 0}
	if err := r.Combat.SupplyIncoming(20, shot, true); err != nil {
		return err
	}
	meta.Instructions = "DEVELOPMENT n6-incoming: paused genuine Single hover; Escape resumes supplied radar-family3 slot20 approaching player0. Observe quadrant/impact feedback and real damage/GPS; normal manual defenses may change guidance. No autonomous launch or forced outcome."
	meta.Expected = "Real family3 motion/contact/chance applies synchronous7468; an impact need not destroy the helicopter. No result/award is supplied."
	meta.Adjustments = append(meta.Adjustments, "Incoming override: source fuel100/countdown150, shared LFSR0280; supplied hostile slot20 flags0043/life30/threshold0/owner1/direct target0/speed306(51*6)/gain64; pose player+[H0:0,H1:4096,height:0], angles[0,8000,0], pending quadrant=true. Owner is existing prepared world row1; no enemy constructor/attack policy executed.")
	p.publishFlight()
	p.resetPresentation()
	return nil
}
