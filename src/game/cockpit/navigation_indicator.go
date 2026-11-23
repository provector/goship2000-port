package cockpit

import "fmt"

// NavigationBearing is the already resolved source INS direction, not a GPS or
// target producer. Inactive (including nil) means source waypoint selector488=0.
// Active callers supply wordNEG(angle2814-4000); drawing never selects a route.
type NavigationBearing struct {
	Active    bool
	Bearing   uint16
	HUDMarker bool `json:",omitempty"` // R2.2.1 presentation; absent for historical films.
}

// drawHUDNavigationMarker is section2:26c7..2799, distinct from the CPIT INS
// pip above and from GPS numbered route vectors. It marks the CURRENT waypoint
// bearing on the heading strip, never a searched-for world's actual troop row.
func (a *Assets) drawHUDNavigationMarker(dst []byte, heading uint16, in *NavigationBearing) error {
	if in == nil || !in.HUDMarker || !in.Active {
		return nil // selector488==0 jumps to279c; no fabricated centered triangle.
	}
	i, ok := a.Sprites["waypoint-marker"]
	if !ok || i.Width != 7 || i.Height != 4 {
		return fmt.Errorf("prepared HUD waypoint marker missing/incompatible")
	}
	// SUB word, MOV AL,AH, CBW, ADD160, then signed clamps96/224.
	x := clamp(160+int(int8(byte((in.Bearing-heading)>>8))), 96, 224)
	// Source1929 flags1 centers only X, keeps Y4 and source00 transparent.
	return blit(dst, i, 0, 0, i.Width, i.Height, x-i.Width/2, 4, true, Full)
}

// drawNavigationIndicator compiles section2:b58..cda. The no-waypoint branch
// still draws the centred strip marker and 000 rather than leaving panel holes.
func (a *Assets) drawNavigationIndicator(dst []byte, d Definition, heading uint16, in *NavigationBearing) error {
	p := func(off int) int { return int(d.Parameters[off/2]) }
	active := in != nil && in.Active
	bearing := uint16(0)
	if active {
		bearing = in.Bearing
	}
	if active {
		if err := a.Number(dst, p(0x48), p(0x4a), int16(bearing/183), 3); err != nil {
			return err
		}
	}
	marker, ok := a.Sprites["bearing-marker"]
	if !ok || marker.Width != 7 || marker.Height != 3 {
		return fmt.Errorf("prepared INS bearing marker missing/incompatible")
	}
	x, y := p(0x44), p(0x46)
	clip := Full
	if active {
		// Source SUB word, MOV AL,AH, CBW, SAR3: a wrapped signed high byte.
		x += int(int16(int8(byte((bearing-heading)>>8))) >> 3)
		x = clamp(x, p(0x3c), p(0x40))
		clip = Rect{p(0x3c), p(0x3e), p(0x40), p(0x42)}
	}
	// 1929 flag1 centres X only; pixel00 preserves the restored panel.
	if err := blit(dst, marker, 0, 0, marker.Width, marker.Height, x-marker.Width/2, y, true, clip); err != nil {
		return err
	}
	if !active {
		return a.Number(dst, p(0x48), p(0x4a), 0, 3)
	}
	return nil
}
