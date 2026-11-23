package cockpit

import "fmt"

func primaryAmmunitionText(s Station, port, starboard byte) string {
	if s.Mode != 3 {
		return fmt.Sprintf("%d", s.Quantity)
	}
	// Section2:013d..0295: a remaining damaged-side pair displays the full
	// retained quantity on the intact side. Healthy halves use signed SAR1.
	if port != 0 && starboard != 0 {
		return "0 - 0"
	}
	if port != 0 {
		return fmt.Sprintf("0 - %d", s.Quantity)
	}
	if starboard != 0 {
		return fmt.Sprintf("%d - 0", s.Quantity)
	}
	return fmt.Sprintf("%d - %d", (s.Quantity>>1)+(s.Quantity&1), s.Quantity>>1)
}

// This pass follows target imagery and notifications, before terminal effects.
func DrawDamageOverlayScaled(dst []byte, a *Assets, in Inputs, scale int) error {
	if in.CameraMode != 0 || in.DamageOverlay == 0 {
		return nil
	}
	return LogicalPass(dst, scale, func(p []byte) error { return drawDamageOverlay(p, a, in) })
}

// Resident11a9..1221 draws the source11x14 crack/bullet-hole after the cockpit.
// Camera0 owns it even when avionics damage suppresses ordinary HUD producers.
func drawDamageOverlay(dst []byte, a *Assets, in Inputs) error {
	if in.CameraMode != 0 || in.DamageOverlay == 0 {
		return nil
	}
	n := 1
	if in.DamageOverlay == 2 {
		n = 2
	}
	for _, p := range in.DamageOverlayPoints[:n] {
		// VGA1929 flags7: both axes centered and zero-transparent.
		if err := a.Sprite(dst, "driver-conditional", int(p[0])-5, int(p[1])-7, Full); err != nil {
			return err
		}
	}
	return nil
}
