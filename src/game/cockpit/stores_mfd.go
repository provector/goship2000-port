package cockpit

import "fmt"

func storesFuelText(value int16) string { return fmt.Sprintf("%d%%", uint16(value)) }

// storesMFD compiles section2:15cc..16e8 and its secondary-panel counterpart.
// Source formatter0000:00e8 style41 is unsigned decimal plus%, not pounds;
// this is runtime SHHD17 fuel percentage, distinct from accepted fuel mass.
func (a *Assets) storesMFD(dst []byte, d Definition, secondary bool, in Inputs) error {
	base := 0x50
	if secondary {
		base = 0x5c
	}
	p := func(off int) int { return int(d.Parameters[off/2]) }
	left, top, right, bottom := p(base+4), p(base+6), p(base+8), p(base+10)
	if left < 0 || top < 0 || right >= 320 || bottom >= 200 || left >= right || top >= bottom {
		return fmt.Errorf("invalid stores MFD panel")
	}
	clip := Rect{left, top, right - 1, bottom - 1}
	text := func(s string, x, y int, style uint16) error {
		return a.Text(dst, s, x, y, 0x1a, style, clip)
	}
	if err := text(a.Literals["fuel"], left+2, top+6, 0x12); err != nil {
		return err
	}
	if err := text(storesFuelText(in.Fuel), right-12, top+6, 0x812); err != nil {
		return err
	}
	for i, s := range in.Stations {
		if !s.displayed() {
			continue
		}
		y := top + 6*(i+2)
		if err := text(s.Name, left+2, y, 0x12); err != nil {
			return err
		}
		if s.Present || s.SystemID > 1 {
			if err := text(fmt.Sprintf("%d", uint16(s.Quantity)), right-12, y, 0x812); err != nil {
				return err
			}
		}
	}
	// Source19a1→VGA0e34 is a destination brightness remap over inclusive
	// bounds. Earlier prose called it a border/clear; it is neither operation.
	// Ordinary panels use base15h after text, alternate E1.bit0 panels skip it.
	if in.SystemFlags&1 == 0 {
		if len(a.ScenePresentation.ConfirmationRemap) != 256 {
			return fmt.Errorf("stores MFD requires selected-VGA brightness remap")
		}
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				at := y*320 + x
				PutLogical(dst, at, a.ScenePresentation.ConfirmationRemap[dst[at]]+0x15)
			}
		}
	}
	return nil
}
