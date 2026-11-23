package menus

import assetdata "gunship2000-assets/game/assets"

// SettingsHelpState is transient source9bba feedback ownership. Draw never
// changes it. A different AREA clears feedback before the source redraw, even
// when the same input also activates the newly reached control.
type SettingsHelpState struct {
	Detailed  bool
	idleTicks int
}

const settingsHelpFeedbackTicks = 600 // Source C686 uses a strict signed >600 callback-count test.

func (s *SettingsHelpState) Observe(previousArea, currentArea int, activated bool) {
	if currentArea != previousArea {
		*s = SettingsHelpState{}
		return
	}
	if activated && currentArea > 0 {
		s.Detailed = true
		s.idleTicks = 0
	}
}

// The existing logical menu clock supplies callback ticks. This does not infer
// a DOS wall-clock duration or change APU readiness/difficulty state.
func (s *SettingsHelpState) Tick() {
	if s.Detailed {
		return
	}
	s.idleTicks++
	if s.idleTicks > settingsHelpFeedbackTicks {
		s.Detailed = true
		s.idleTicks = 0
	}
}

// Original0c87:0578 wraps at the last space or hyphen, including that byte in
// the emitted substring. It does not normalize whitespace with strings.Fields.
// Only the bounded AREA12/default wrapping mode is needed for these literals.
func settingsHelpLines(font *assetdata.BitmapFont, text string, width int) []string {
	if font == nil || width < 1 {
		return nil
	}
	raw := []byte(text)
	for i, c := range raw {
		if c == 0 {
			raw = raw[:i]
			break
		}
	}
	var lines []string
	for len(raw) > 0 {
		n, lastBreak, used := 0, 0, 0
		newline, overflow := false, false
		for n < len(raw) {
			c := raw[n]
			if c == '\n' {
				newline = true
				break
			}
			if c == ' ' || c == '-' {
				lastBreak = n + 1
			}
			advance := font.Measure(raw[n : n+1])
			if used+advance > width {
				overflow = true
				break
			}
			used += advance
			n++
		}
		count := n
		if overflow && lastBreak > 0 {
			count = lastBreak
		}
		if count == 0 && !newline {
			count = 1
		} // Source fixtures have no over-wide single glyph; guarantee diagnostic progress.
		lines = append(lines, string(raw[:count]))
		if newline {
			raw = raw[n+1:]
		} else {
			raw = raw[count:]
		}
	}
	return lines
}
