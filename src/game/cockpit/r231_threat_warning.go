package cockpit

// Source section2:3C34..3CC2 remaps only the CPIT warning-label ink. The
// palette operands are CPIT9A/9B; they are not a generic red rectangle.
func drawThreatWarningIndicators(dst []byte, d Definition, flags, phase uint16) {
	for _, v := range []struct {
		offset int
		active bool
	}{
		{0x6c, flags&4 != 0},
		{0x74, flags&0x40 != 0 || flags&0x2000 != 0 && phase&1 != 0},
		{0x70, flags&0x80 != 0},
	} {
		if v.active {
			exactReplace(dst, int(d.Parameters[v.offset/2]), int(d.Parameters[(v.offset+2)/2]), 10, 6, d.Colours[4], d.Colours[5])
		}
	}
}
