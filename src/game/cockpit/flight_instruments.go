package cockpit

// drawAlternateIndicators compiles section2:0a20..0b58's selected-VGA path.
// 196a is an exact-index rectangle replacement, not a solid/alpha rectangle.
// E1 bit0 chooses these instead of the four analog needles/small attitude gauge.
func drawAlternateIndicators(dst []byte, d Definition, in Inputs) {
	p := func(off int) int { return int(d.Parameters[off/2]) }
	if in.SystemFlags&2 != 0 {
		exactReplace(dst, p(0x20), p(0x22), 16, 7, 0x47, 0x1f)
	}
	// Source highlights gear-down, or blinks while retracted at low clearance.
	if in.SystemFlags&4 == 0 || in.HUDScale < 64 && in.BlinkPhase&1 != 0 {
		exactReplace(dst, p(0x28), p(0x2a), 17, 7, 0xef, 0x96)
	}
	x, y, h := p(0x30), p(0x32), 5
	if in.Lateral != 0 {
		h = 3
		if in.Lateral < 0 {
			y += 2
		}
	}
	exactReplace(dst, x, y, 5, h, 0x45, 0x0a)
}

func (a *Assets) alternateAttitude(dst []byte, d Definition, matrix [3]int16) error {
	cx, cy := int(d.Parameters[0x4c/2]), int(d.Parameters[0x4e/2])
	clip := Rect{cx - 20, cy - 20, cx + 20, cy + 20}
	// Section2:17f3 passes flags3: center both axes, preserve index00.
	i, ok := a.Sprites["attitude-overlay"]
	if !ok {
		return a.Sprite(dst, "attitude-overlay", cx, cy, clip)
	}
	if err := blit(dst, i, 0, 0, i.Width, i.Height, cx-i.Width/2, cy-i.Height/2, true, clip); err != nil {
		return err
	}
	a.attitudeClipped(dst, d, matrix, clip)
	return nil
}
