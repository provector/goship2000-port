package cockpit

// clearanceNumber is section2:3ce2. The original unsigned width decision
// precedes the number helper's signed absolute value; negative ticks stay3digit.
func (a *Assets) clearanceNumber(dst []byte, x, y int, value int16) error {
	style := uint16(0x103)
	if uint16(value) >= 1000 && uint16(value) <= 0x8000 {
		style = 0x104
		x -= 4
	}
	return a.Number(dst, x, y, value, style)
}

func (a *Assets) clearanceNumbers(dst []byte, clearance int16) error {
	base := int16((clearance >> 6) * 50)
	phase := int((clearance & 0x3c) >> 2)
	for _, tick := range []struct {
		y     int
		value int16
	}{
		{54, base}, {38, base + 50}, {70, base - 50}, {22, base + 100},
	} {
		if err := a.clearanceNumber(dst, 286, tick.y+phase, tick.value); err != nil {
			return err
		}
	}
	if err := a.Sprite(dst, "hud-arrow-alternate", 285, 53, Full); err != nil {
		return err
	}
	return a.clearanceNumber(dst, 286, 54, int16(uint16(clearance*25)>>5))
}

// speedNumber consumes late source0476, not world velocity or VSI/E4. These
// are word arithmetic and source blink predicates; drawing never advances DA.
func (a *Assets) speedNumber(dst []byte, speed int16, phase uint16) error {
	name, boxY, numberY, value := "hud-arrow-alternate", 57, 58, speed
	if speed > 250 || speed < -125 {
		if phase&1 == 0 {
			return nil
		}
		name = "hud-arrow"
		if speed > 250 {
			boxY, numberY, value = 17, 18, 250
		} else {
			boxY, numberY, value = 77, 78, 125
		}
	} else {
		if speed < 0 && phase&2 == 0 {
			return nil
		}
		q := int(int16(speed<<2) / 25)
		boxY, numberY = 57-q, 58-q
	}
	if err := a.Sprite(dst, name, 19, boxY, Full); err != nil {
		return err
	}
	return a.Number(dst, 20, numberY, value, 0x103)
}

func (a *Assets) drawDefenseCounts(dst []byte, d Definition, in DefenseInputs, phase uint16) error {
	for _, c := range []struct {
		offset       int
		count, timer int16
	}{
		{0x8c, in.Chaff, in.ChaffTimer}, {0x90, in.Flares, in.FlareTimer},
	} {
		if c.timer != 0 && phase&2 == 0 {
			continue
		}
		if err := a.Number(dst, int(d.Parameters[c.offset/2]), int(d.Parameters[c.offset/2+1]), c.count, 0x202); err != nil {
			return err
		}
	}
	return nil
}
