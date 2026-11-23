package menus

import assetdata "gunship2000-assets/game/assets"

// Source GS20F4:729E shell shared by76A2 Delete and74B2 Abort. It is an
// editor-palette owner, distinct from the Mission Files1C07 confirmation.
func (c *FrontEnd) drawEditorDialogShell(frame []byte, title string) {
	assetdata.OfficeFill(frame, 79, 59, 163, 93, 240)
	for _, r := range []struct {
		x0, y0, x1, y1 int
		ink            byte
	}{{80, 60, 239, 60, 8}, {80, 60, 80, 149, 8}, {81, 150, 240, 150, 15}, {240, 61, 240, 150, 15}, {85, 70, 235, 70, 15}, {87, 71, 233, 71, 8}} {
		_ = assetdata.OfficeLine(frame, r.x0, r.y0, r.x1, r.y1, r.ink)
	}
	c.editorDialogCentered(frame, 5, title, 63, 0x44, 0)
}
func (c *FrontEnd) editorDialogCentered(frame []byte, font int, text string, y, ink, bias int) {
	f := c.editorAssets().Fonts[font]
	if f == nil {
		return
	}
	width := f.Measure([]byte(text))
	_ = f.DrawClipped(frame, []byte(text), 82+(160-width)/2+bias, y, ink, 0, 0, [4]int{82, 60, 160, 89})
}
func (c *FrontEnd) drawEditorDeleteConfirm(frame []byte) {
	a, e := c.editorAssets(), c.Editor
	c.drawEditorDialogShell(frame, a.Text["delete_title"])
	//737C uses font2 already selected by76A2: black at82/y then colored
	//at83/y+1 within the same centered160x8 text span. No word-wrap occurs.
	for _, r := range []struct {
		key    string
		y, ink int
	}{{"delete1", 80, 255}, {"delete2", 88, 253}} {
		c.editorDialogCentered(frame, 2, a.Text[r.key], r.y, 0, 0)
		c.editorDialogCentered(frame, 2, a.Text[r.key], r.y+1, r.ink, 1)
	}
	if e.ControlChoice >= 0 && e.ControlChoice < len(a.DeleteLabels) {
		c.editorDialogCentered(frame, 3, a.DeleteLabels[e.ControlChoice], 104, 10, 0)
	}
	for _, q := range []struct {
		key, label string
		hot        int
	}{{"abort_no", "NO!", 19}, {"abort_yes", "YES", 18}} {
		r := a.ExitRects[q.key]
		drawEditorRaisedBevel(frame, r)
		f := a.Fonts[5]
		if f == nil {
			continue
		}
		width := f.Measure([]byte(q.label))
		c.clippedText(frame, 5, q.label, r[0]+(r[2]-width)/2, 138, 4, r)
		c.clippedText(frame, 5, q.label[:1], r[0]+q.hot, r[1]+4, 0x44, r)
	}
}
