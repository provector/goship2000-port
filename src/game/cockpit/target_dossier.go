package cockpit

import (
	"fmt"
	"strings"
)

// GS2 section5:458d..4687. Its resident851:0d3b thunk selects overlay5.
// Actual OBJECT+12 indexes name, description, best and worst from an8-byte row.
func DrawTargetDossierScaled(dst []byte, a *Assets, scale, centerX int, name, description, best, worst string) error {
	if a == nil || len(a.ScenePresentation.ConfirmationRemap) != 256 {
		return fmt.Errorf("target dossier requires selected-VGA brightness remap")
	}
	return LogicalPass(dst, scale, func(p []byte) error {
		left := centerX - 110
		// Source45B3 calls19A1: remap the existing scene, not an opaque fill.
		for y := 16; y <= 88; y++ {
			for x := max(0, left); x <= min(319, left+220); x++ {
				at := y*320 + x
				PutLogical(p, at, a.ScenePresentation.ConfirmationRemap[p[at]]+0xc8)
			}
		}
		// Source45D8 passes left+8 as the font window's left edge. LF and
		// wrapped continuation lines return here, including BEST/WORST values.
		clip := Rect{left + 8, 16, left + 220, 88}
		for _, v := range []struct {
			text  string
			x, y  int
			ink   byte
			style uint16
		}{
			{"BEST CHOICE:", left + 14, 56, 2, 0x12}, {"WORST CHOICE:", left + 8, 72, 2, 0x12},
			{name, left + 8, 20, 10, 0x112}, {description, left + 8, 32, 10, 0x112},
			{best, left + 72, 56, 10, 0x112}, {worst, left + 72, 72, 10, 0x112},
		} {
			text, style := v.text, v.style
			if style&0x100 != 0 {
				var err error
				text, err = dossierWrappedText(a, text, v.x, clip)
				if err != nil {
					return err
				}
				style &^= 0x100
			}
			if err := a.Text(p, text, v.x, v.y, v.ink, style, clip); err != nil {
				return err
			}
		}
		return nil
	})
}

// Original common7E0:00DE (body7EDE..7F27) changes LF to spaces, sums actual
// proportional advances, and at X>=right replaces the preceding space withLF.
// After a break it restarts at the font window's left edge, not the first X.
// Keep this source recipe local: historical cockpit Text interpretation stays
// unchanged. Unsupported no-space overflow is bounded rather than reading
// before the caller's buffer as the original unchecked rewind could do.
func dossierWrappedText(a *Assets, text string, startX int, clip Rect) (string, error) {
	b := []byte(strings.SplitN(text, "\x00", 2)[0])
	x, lineStart := int16(startX), 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			b[i] = ' '
		}
		glyph := int(b[i]) - a.Font.First
		if glyph < 0 || glyph >= len(a.Font.Advances) {
			return "", fmt.Errorf("target dossier unsupported text byte%02x", b[i])
		}
		x += int16(a.Font.Advances[glyph])
		if x < int16(clip.Right) {
			continue
		}
		at := i
		for at >= lineStart && b[at] != ' ' {
			at--
		}
		if at < lineStart {
			return "", fmt.Errorf("target dossier word exceeds source font window")
		}
		b[at] = '\n'
		i = at
		lineStart = at + 1
		x = int16(clip.Left)
	}
	return string(b), nil
}
