package assets

import "strings"

const TooltipInset = 2

// Bottom-strip UI only. It never handles document/body, dialog or HUD text.
func DrawBottomTooltip(frame []byte, font *BitmapFont, text string, colour int) {
	if len(frame) != 64000 {
		return
	}
	OfficeFill(frame, 0, 191, 320, 9, 0)
	if font == nil {
		return
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\r", " ")
	raw := []byte(text)
	width := 320 - 2*TooltipInset
	if font.Measure(raw) > width {
		suffix := []byte("...")
		for len(raw) > 0 && font.Measure(raw)+font.Measure(suffix) > width {
			raw = raw[:len(raw)-1]
		}
		raw = append(raw, suffix...)
	}
	_ = font.DrawClipped(frame, raw, TooltipInset+(width-font.Measure(raw))/2, 193, colour, 0, 0, [4]int{TooltipInset, 191, width, 9})
}
