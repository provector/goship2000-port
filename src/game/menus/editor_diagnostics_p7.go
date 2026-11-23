package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"strings"
)

// This explicitly native diagnostic reader preserves the source one-line status
// owner. It neither edits a document nor replaces the original '?' model help.
func (c *FrontEnd) editorDiagnosticLines(text string) []string {
	f := c.editorAssets().Fonts[3]
	if f == nil {
		return []string{text}
	}
	lines := []string{}
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, r := range paragraph {
			next := line + string(r)
			if line != "" && f.Measure([]byte(next)) > 286 {
				lines = append(lines, line)
				line = string(r)
			} else {
				line = next
			}
		}
		lines = append(lines, line)
	}
	return lines
}
func (c *FrontEnd) EditorDiagnosticPages(text string) int {
	return max(1, (len(c.editorDiagnosticLines(text))+15)/16)
}
func (c *FrontEnd) drawEditorDiagnostics(frame []byte, text string, page int) {
	assetdata.OfficeFill(frame, 10, 10, 300, 181, 240)
	drawEditorRaisedBevel(frame, [4]int{10, 10, 299, 180})
	c.clippedText(frame, 5, "MISSION BUILDER DIAGNOSTIC", 17, 17, 0x44, [4]int{16, 15, 288, 14})
	lines := c.editorDiagnosticLines(text)
	pages := max(1, (len(lines)+15)/16)
	page = max(0, min(pages-1, page))
	start := page * 16
	for n := 0; n < 16 && start+n < len(lines); n++ {
		c.clippedText(frame, 3, lines[start+n], 17, 35+n*8, 8, [4]int{16, 34, 288, 130})
	}
	c.clippedText(frame, 3, fmt.Sprintf("%d/%d  ARROWS: PAGE   ENTER/ESC: RETURN", page+1, pages), 17, 177, 8, [4]int{16, 176, 288, 10})
}
