package cockpit

import "fmt"

type ContactPoint struct {
	X, Y   int16
	Colour byte
	Attack bool
}

// ContactAvionicsInputs is a snapshot, not access to world/entity state. Incoming
// projectile plots are copied from the reached logical display phase.
type ContactAvionicsInputs struct {
	TargetActive     bool
	TargetBearing    uint16
	ThreatZoom       uint16
	ThreatPoints     []ContactPoint
	ProjectilePoints []ProjectilePoint
	WarningFlags     uint16 `json:",omitempty"` // Reached3586/3C2B accumulator; zero is omitted for old films.
}

// DrawContactAvionics composes contacts then the source24-slot projectile phase.
// It must remain pure: source zoom/warning side effects are logical-owner inputs.
func (a *Assets) DrawContactAvionics(dst []byte, in Inputs) error {
	v := in.ContactAvionics
	if v == nil || !in.HUD {
		return nil
	}
	if !LogicalTarget(dst) {
		return fmt.Errorf("contact HUD needs320x200 indexed target")
	}
	if in.Damage[0]&0x80 == 0 && v.TargetActive {
		heading := uint16(-uint16(in.Heading + uint16(in.HeadingPerturbation)))
		x := clamp(160+int(int8(byte((v.TargetBearing-heading)>>8))), 96, 224)
		img, ok := a.Sprites["target-marker"]
		if !ok {
			return fmt.Errorf("prepared target marker missing")
		}
		if err := blit(dst, img, 0, 0, img.Width, img.Height, x-img.Width/2, 4, true, Full); err != nil {
			return err
		}
	}
	if in.CameraMode != 0 || in.Arrangement == 2 || in.SecondaryMFD != 5 || in.Damage[1]&0x40 != 0 {
		return nil
	}
	if err := a.DrawThreatBackground(dst, in.AircraftID, v.ThreatZoom); err != nil {
		return err
	}
	for _, q := range v.ThreatPoints {
		a.DrawThreatContactPoint(dst, in.AircraftID, in.BlinkPhase, q)
	}
	for _, q := range v.ProjectilePoints {
		a.DrawThreatProjectilePoint(dst, in.AircraftID, q)
	}
	f, err := Family(in.AircraftID)
	if err != nil {
		return err
	}
	drawThreatWarningIndicators(dst, a.Definitions[f], v.WarningFlags, in.BlinkPhase)
	return nil
}

// DrawThreatBackground writes the source35d9..3667 markers before contact
// readback. Logical feedback and physical rendering use the same pure operation.
// The caller owns the reached-view gate and supplies the pre-update zoom.
func (a *Assets) DrawThreatBackground(dst []byte, aircraftID int, zoom uint16) error {
	if !LogicalTarget(dst) {
		return fmt.Errorf("threat background needs320x200 indexed target")
	}
	f, err := Family(aircraftID)
	if err != nil {
		return err
	}
	d := a.Definitions[f]
	p := func(off int) int { return int(d.Parameters[off/2]) }
	clip := Rect{p(0x60), p(0x62), p(0x64), p(0x66)}
	cx, cy := p(0x5c), p(0x5e)
	pixel(dst, cx, cy, 10, clip)
	for i := 0; i < int(zoom)+1; i += 2 {
		pixel(dst, clip.Left+i, cy, 2, clip)
		pixel(dst, clip.Right-i, cy, 2, clip)
		pixel(dst, cx, clip.Top+i, 2, clip)
		pixel(dst, cx, clip.Bottom-i, 2, clip)
	}
	return nil
}
