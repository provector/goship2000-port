package menus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

func (c *FrontEnd) editorIconCategory(p gamestate.AuthoredPlacement) int {
	w, err := gamestate.AuthoredWorld(c.editorData(), c.Editor.Document.World)
	if err != nil || p.Component < 0 || p.Component*8+1 >= len(w.FRMD) {
		return -1
	}
	model := int(w.FRMD[p.Component*8+1])
	types := c.editorData().Tables.EditorObjectTypes
	if model*27+1 >= len(types) {
		return -1
	}
	return min(int(types[model*27+1]), 9)
}
func (c *FrontEnd) editorIcon(frame []byte, index, x, y int, shadow bool) {
	c.editorIconRow(frame, index, x, y, shadow, 0)
}

func (c *FrontEnd) editorIconRow(frame []byte, index, x, y int, shadow bool, rowOffset int) {
	a := c.editorAssets()
	if index < 0 || index >= len(a.IconRects) {
		return
	}
	r := a.IconRects[index]
	r[1] += rowOffset
	anchor := a.IconAnchors[index]
	clip := c.editorMapBounds()
	for sy := 0; sy < r[3]; sy++ {
		for sx := 0; sx < r[2]; sx++ {
			v := a.Icons[(r[1]+sy)*320+r[0]+sx]
			if v == 0 {
				continue
			}
			dx, dy := x+sx-anchor[0], y+sy-anchor[1]
			if dx < 0 || dx >= 320 || dy < 0 || dy >= 200 || !editorInclusive(dx, dy, clip) {
				continue
			}
			if shadow {
				v = 8
			}
			frame[dy*320+dx] = v
		}
	}
}
func (c *FrontEnd) editorFrame() []byte {
	a, e := c.editorAssets(), c.Editor
	if a == nil || e == nil {
		return make([]byte, 64000)
	}
	if e.Stage == "world" {
		frame := append([]byte(nil), a.Background...)
		for i, name := range e.Worlds {
			p := a.Previews[editorMapKey(name)]
			for y := 0; y < p.Height; y++ {
				for x := 0; x < p.Width; x++ {
					dx, dy := p.X+x, p.Y+y
					if dx >= 0 && dx < 320 && dy >= 0 && dy < 200 {
						frame[dy*320+dx] = p.Pixels[y*p.Width+x]
					}
				}
			}
			if i == e.WorldIndex {
				_ = assetdata.OfficeLine(frame, p.X-1, p.Y-1, p.X+p.Width, p.Y-1, 0x44)
				_ = assetdata.OfficeLine(frame, p.X-1, p.Y+p.Height, p.X+p.Width, p.Y+p.Height, 0x44)
			}
		}
		c.centered(frame, 5, a.Text["map_title"], 180, 0x44)
		return frame
	}
	frame := append([]byte(nil), a.Panel...)
	m, ok := c.documentEditorMap(e.Document)
	if ok && len(m.Pixels) == 64000 {
		for y := 0; y < 130; y++ {
			copy(frame[y*320:y*320+166], m.Pixels[y*320:y*320+166])
		}
	}
	// Source panel fields use the actual world palette's source indices. Restore
	// from the immutable panel on every draw; no stale overlays or broad clears.
	assetdata.OfficeFill(frame, 171, 2, 145, 7, 202)
	c.clippedText(frame, 3, e.Document.Title, 172, 3, 11, [4]int{171, 2, 145, 7})
	c.drawEditorParameters(frame)
	label := "DAY"
	if e.Document.Conditions.Flags&0x200 != 0 {
		label = "NIGHT"
		if w, err := gamestate.AuthoredWorld(c.editorData(), e.Document.World); err == nil && w.Theatre == 2 {
			label = "DUSK"
		}
	}
	assetdata.OfficeFill(frame, 199, 76, 112, 7, 202)
	c.clippedText(frame, 3, label, 199, 76, 15, [4]int{199, 76, 112, 7})
	temperatureBG, windBG := 0, 0
	if e.Stage == "numeric" {
		if e.Numeric == "temperature" {
			temperatureBG = 12
		} else {
			windBG = 12
		}
	}
	c.editorDigitsBackground(frame, int(e.Document.Conditions.Temperature), 238, 86, 3, temperatureBG)
	c.editorDigitsBackground(frame, e.Document.Conditions.WindSpeed, 200, 96, 2, windBG)
	c.editorDigitsBackground(frame, editorWindDegrees(e.Document.Conditions.WindDirection), 258, 96, 3, windBG)
	// Source2f87..2feb uses16 bearings; arrow4b2 and compass text have
	// separate owners. Convert the reached stored turn value, never host angle.
	sector := (((int(e.Document.Conditions.WindDirection) >> 8) + 8) & 0xf0) >> 4
	if len(a.WindVectors) == 16 {
		v := a.WindVectors[sector]
		assetdata.OfficeFill(frame, 281, 97, 7, 3, 0)
		assetdata.OfficeFill(frame, 282, 96, 5, 5, 0)
		_ = assetdata.OfficeLine(frame, 284, 98, 284+v[0], 98+v[1], 0x44)
	}
	assetdata.OfficeFill(frame, 291, 96, 20, 5, 202)
	labels := c.editorData().Tables.Compass
	if sector < len(labels) {
		c.clippedText(frame, 3, labels[sector], 291, 96, 15, [4]int{291, 96, 20, 5})
	}

	vis := "EXCELLENT"
	if e.Document.Conditions.Flags&0x100 != 0 {
		vis = "POOR"
	}
	assetdata.OfficeFill(frame, 228, 106, 88, 7, 202)
	c.text(frame, 3, vis, 228, 106, 15)
	displayDocument := e.Document
	if e.SupportCancelled != nil && e.Document.SupportMemory == nil {
		q := *e.Document
		q.Support = e.SupportCancelled
		displayDocument = &q
	}
	remaining, _ := gamestate.AuthoredBudget(c.editorData(), displayDocument)
	c.editorDigits(frame, remaining, 225, 132, 2)
	counts, countErr := gamestate.AuthoredDefenderCounts(c.editorData(), displayDocument)
	if countErr == nil {
		for i, count := range counts {
			c.editorDigits(frame, count, 181+(i/5)*76, 142+(i%5)*9, 2)
		}
	}

	//20f4:597e/540a draws the fixed overview independently of the
	//detail-map callback2114. Opening C020 must not redirect the overview's
	//registry through the binary32 detail transform or clip.
	e.paintOverview = true
	c.drawEditorMapOverlays(frame)
	c.drawEditorPlacedSymbols(frame, true)
	e.paintOverview = false
	c.drawEditorDetail(frame)
	if e.View.Detail {
		//54A7 weapon effectiveness is the overview callback's pixel-radius
		//owner. Detail230E draws all ordinary atlas icons independently of
		//the overview category registry; only the reached route pass is shared.
		if e.Paths || e.All {
			c.drawEditorPaths(frame)
		}
		c.drawEditorPlacedSymbols(frame, false)
	}
	if e.Held && e.Shadow != nil {
		p := *e.Shadow
		index := e.Category - 1
		switch e.Role {
		case "base":
			index = 10
		case "FARP":
			index = 11
		case "primary":
			index = 12
		case "secondary":
			index = 13
		}
		x, y := c.editorScreen(p.X, p.Y)
		r := c.editorMapBounds()
		_ = assetdata.OfficeLine(frame, max(r[0], min(r[0]+r[2], e.CursorX)), max(r[1], min(r[1]+r[3], e.CursorY)), max(r[0], min(r[0]+r[2], x)), max(r[1], min(r[1]+r[3], y)), 8)
		c.editorIcon(frame, index, x, y, true)
	}
	c.drawEditorReference(frame)
	//370's map status belongs to the underlying editor bank. Modal729e and
	//the caller-relative popups are later draws; repainting status over them
	//erases their left border/rows at148..153.
	if e.Status != "" {
		c.drawEditorStatus(frame, e.Status)
	}
	c.drawEditorControlOverlay(frame)
	if e.Stage == "kind" || e.Stage == "option" || e.Stage == "support" {
		c.drawEditorOptions(frame)
	}
	if e.Stage == "exit" {
		c.drawEditorExit(frame)
	}
	if e.Stage == "abort" {
		c.drawEditorAbort(frame)
	}
	if e.Stage == "title" {
		drawPreflightFrame(c.Office.Assets.Missions, frame, [4]int{60, 78, 200, 38}, 0, 0)
		c.centered(frame, 3, "MISSION NAME", 84, 11)
		c.text(frame, 5, e.Text+"_", 70, 98, 15)
	}
	if e.Diagnostics {
		c.drawEditorDiagnostics(frame, e.DiagnosticText, e.DiagnosticsPage)
	}
	return frame
}
func (c *FrontEnd) drawEditorStatus(frame []byte, text string) {
	assetdata.OfficeFill(frame, 4, 148, 148, 6, 0)
	c.clippedText(frame, 3, text, 5, 148, 13, [4]int{4, 148, 148, 6})
}
func editorSourceCoordinates(x, y int) string { return fmt.Sprintf("%02d/%02d", x/96, 16+y/-72) }
func (c *FrontEnd) editorDigits(frame []byte, value, x, y, n int) {
	c.editorDigitsBackground(frame, value, x, y, n, 0)
}
func (c *FrontEnd) editorDigitsBackground(frame []byte, value, x, y, n, background int) {
	//406's signed IDIV remainder is printed in absolute value; sign is ink44.
	ink := 15
	if value < 0 {
		ink = 0x44
	}
	s := fmt.Sprintf("%0*d", n, absInt(value))
	if len(s) > n {
		s = s[len(s)-n:]
	}
	//20f4:0438..0456 fills the numeric owner with its caller's color. The
	//panel contains baked80/00: copying it then drawing transparent digits
	//unions the old and new masks. Counts/ordinary values pass0; an active
	//source numeric edit passes12. Neither caller owns the adjacent label/icon.
	assetdata.OfficeFill(frame, x-1, y, n*5+1, 5, byte(background))
	for i, b := range []byte(s) {
		c.clippedText(frame, 3, string(b), x+i*5, y, ink, [4]int{x + i*5, y, 5, 5})
	}
}

// singleton in the private editor table. These are neither FMHD site names nor
// OBJDBASE's model-information prose. Keep the display lookup bounded even for
// a malformed detached draft; validation remains the launch admission owner.
//
//1abf:13de selects FORM.DAT+1 for a formation and OBJTYPES.DAT+2 for a
func (c *FrontEnd) editorPlacementCaption(p gamestate.AuthoredPlacement, formationOnly bool) string {
	w, err := gamestate.AuthoredEditorWorld(c.editorData(), c.Editor.Document.World)
	if err != nil || p.Formation < 0 || p.Formation >= len(w.FMHD)/39 || p.Component < 0 || p.Component >= len(w.FRMD)/8 {
		return ""
	}
	terminated := func(b []byte) string {
		if i := bytes.IndexByte(b, 0); i >= 0 {
			b = b[:i]
		}
		return string(b)
	}
	if formationOnly || w.FRMD[p.Component*8+2] == 1 {
		//4c3c formats the selected world filename's first four bytes, as
		//carried byB83F. THHD+1 is a long theatre title, not this file prefix.
		if len(w.WorldName) < 4 {
			return ""
		}
		b := c.editorAssets().FormationTypes[w.WorldName[:4]]
		id := int(binary.LittleEndian.Uint16(w.FMHD[p.Formation*39+25:]))
		if id < 0 || id >= len(b)/26 {
			return ""
		}
		return terminated(b[id*26+1 : (id+1)*26])
	}
	id := int(w.FRMD[p.Component*8+1])
	b := c.editorData().Tables.EditorObjectTypes
	if id >= len(b)/27 {
		return ""
	}
	return terminated(b[id*27+2 : (id+1)*27])
}

func (c *FrontEnd) drawEditorParameters(frame []byte) {
	r := [4]int{214, 66, 80, 6}
	assetdata.OfficeFill(frame, r[0], r[1], r[2], r[3], 202)
	name := "None"
	if c.Editor.SupportCancelled != nil {
		name = c.Editor.SupportCancelName
	}
	if d := c.Editor.Document; gamestate.AuthoredSupportCaption(d) != nil {
		selected := gamestate.AuthoredSupportCaption(d)
		p := gamestate.AuthoredPlacement{Formation: selected.Formation, Component: selected.Component, Template: selected.Template}
		if caption := c.editorPlacementCaption(p, selected.Kind == 0x19); caption != "" {
			name = caption
		}
	}
	c.clippedText(frame, 3, name, r[0], r[1], 15, r)
	for _, p := range c.Editor.Document.Placements {
		switch p.Role {
		case "base":
			//20f4:1e70: the120x6 area starts198,12, not the label at172.
			r := [4]int{198, 12, 120, 6}
			assetdata.OfficeFill(frame, r[0], r[1], r[2], r[3], 202)
			text := c.editorPlacementCaption(p, true) + " l" + c.editorAssets().Text["located_tail"] + editorSourceCoordinates(p.X, p.Y)
			c.clippedText(frame, 3, text, r[0], r[1], 15, r)
		case "FARP":
			//20f4:1da8 uses the separate five-row FARP owner.
			r := [4]int{198, 22, 120, 5}
			assetdata.OfficeFill(frame, r[0], r[1], r[2], r[3], 202)
			text := "L" + c.editorAssets().Text["located_tail"] + editorSourceCoordinates(p.X, p.Y)
			c.clippedText(frame, 3, text, r[0], r[1], 15, r)
		case "primary", "secondary":
			x, y := 214, 32
			if p.Role == "secondary" {
				x, y = 224, 49
			}
			r := [4]int{x, y, 80, 6}
			assetdata.OfficeFill(frame, r[0], r[1], r[2], r[3], 202)
			if p.Kind >= 0 && p.Kind < len(c.editorAssets().GoalLabels) {
				c.clippedText(frame, 3, c.editorAssets().GoalLabels[p.Kind], x, y, 15, r)
			}
			//1abf:1472..14a8 deliberately uses+15, unlike Base/FARP's+16.
			//Its printf%c swaps fore68 with alternate15; it consumes no vararg
			//and is not a printed character. The native font carries this exact
			//inline ink change with a high-bit color byte, without global state.
			r = [4]int{178, y + 8, 130, 6}
			assetdata.OfficeFill(frame, r[0], r[1], r[2], r[3], 202)
			coords := fmt.Sprintf("%02d/%02d", p.X/96, 15+p.Y/-72)
			text := c.editorPlacementCaption(p, false) + " " + string([]byte{0x8f}) + "at " + coords
			c.clippedText(frame, 3, text, r[0], r[1], 0x44, r)
		}
	}
}
func (c *FrontEnd) drawEditorOptions(frame []byte) {
	e := c.Editor
	labels := []string{}
	if e.Stage == "kind" {
		labels = c.editorAssets().GoalLabels
	} else {
		for _, o := range e.Options {
			labels = append(labels, o.Name)
		}
	}
	c.drawEditorPopup(frame, labels)
}
func (c *FrontEnd) drawEditorPopup(frame []byte, labels []string) {
	e := c.Editor
	n := min(len(labels), 36)
	if n == 0 {
		return
	}
	rows := (n + 1) / 2
	assetdata.OfficeFill(frame, e.PopupX, e.PopupBottom-8*rows-1, 189, 8*rows+1, 7)
	for i := 0; i < n; i++ {
		r := e.popupArea(i, n)
		bg := byte(8)
		if i == e.PopupHover {
			bg = 2
		}
		assetdata.OfficeFill(frame, r[0], r[1], 90, 7, bg)
		c.clippedText(frame, 3, labels[i], r[0], r[1], 10, [4]int{r[0], r[1], 90, 7})
	}
}
func (c *FrontEnd) drawEditorExit(frame []byte) {
	a, e := c.editorAssets(), c.Editor
	// Original20f4:729e border/fill, not a menu palette upload.
	assetdata.OfficeFill(frame, 79, 59, 163, 93, 240)
	for _, r := range []struct {
		x0, y0, x1, y1 int
		ink            byte
	}{{80, 60, 239, 60, 8}, {80, 60, 80, 149, 8}, {81, 150, 240, 150, 15}, {240, 61, 240, 150, 15}, {85, 70, 235, 70, 15}, {87, 71, 233, 71, 8}} {
		_ = assetdata.OfficeLine(frame, r.x0, r.y0, r.x1, r.y1, r.ink)
	}
	print := func(font int, text string, y, ink int) {
		f := a.Fonts[font]
		if f == nil {
			return
		}
		width := f.Measure([]byte(text))
		_ = f.DrawClipped(frame, []byte(text), 82+(160-width)/2, y, ink, 0, 0, [4]int{82, 60, 160, 89})
	}
	print(5, a.Text["exit_title"], 63, 0x44)
	valid := !e.supportCancellationGuarded() && gamestate.ValidateAuthored(c.editorData(), e.Document) == nil
	if valid {
		print(2, a.Text["valid"], 124, 10)
	} else {
		print(2, a.Text["invalid"], 124, 0x44)
	}
	roles := map[string]bool{}
	for _, p := range e.Document.Placements {
		roles[p.Role] = true
	}
	y := 75
	for _, v := range []struct{ role, key string }{{"base", "base_missing"}, {"primary", "primary_missing"}, {"secondary", "secondary_missing"}, {"FARP", "farp_missing"}} {
		if !roles[v.role] {
			print(2, a.Text[v.key], y, 8)
			y += 10
		}
	}
	for key, r := range c.editorExitAreas(valid) {
		drawEditorRaisedBevel(frame, r)
		label := a.Text[key]
		w := c.Menus.Fonts[5].Measure([]byte(label))
		c.clippedText(frame, 5, label, r[0]+(r[2]-w)/2, 138, 4, r)
		if len(label) > 0 {
			c.clippedText(frame, 5, label[:1], r[0]+(r[2]-w)/2, 138, 0x44, r)
		}
	}
}

func (c *FrontEnd) drawEditorAbort(frame []byte) {
	a := c.editorAssets()
	assetdata.OfficeFill(frame, 79, 59, 163, 93, 240)
	for _, r := range []struct {
		x0, y0, x1, y1 int
		ink            byte
	}{{80, 60, 239, 60, 8}, {80, 60, 80, 149, 8}, {81, 150, 240, 150, 15}, {240, 61, 240, 150, 15}, {85, 70, 235, 70, 15}, {87, 71, 233, 71, 8}} {
		_ = assetdata.OfficeLine(frame, r.x0, r.y0, r.x1, r.y1, r.ink)
	}
	print := func(font int, text string, y, ink int) {
		f := a.Fonts[font]
		if f == nil {
			return
		}
		w := f.Measure([]byte(text))
		_ = f.DrawClipped(frame, []byte(text), 82+(160-w)/2, y, ink, 0, 0, [4]int{82, 60, 160, 89})
	}
	print(5, a.Text["abort_title"], 63, 0x44)
	print(2, a.Text["abort1"], 80, 8)
	print(2, a.Text["abort2"], 88, 8)
	print(3, a.Text["abort3"], 104, 10)
	print(3, a.Text["abort4"], 116, 0x44)
	for i, label := range []string{"NO!", "YES"} {
		r := a.ExitRects[[]string{"abort_no", "abort_yes"}[i]]
		drawEditorRaisedBevel(frame, r)
		w := c.Menus.Fonts[5].Measure([]byte(label))
		c.clippedText(frame, 5, label, r[0]+(60-w)/2, 138, 4, r)
	}
}

// Editor20f4:05bc toggles a four-line 8/15 bevel over the existing bank.
// It never fills with a POINTER tile's menu color (which hides red labels).
func drawEditorBevel(frame []byte, r [4]int) {
	drawEditorButtonBevel(frame, r, true)
}

func drawEditorRaisedBevel(frame []byte, r [4]int) {
	drawEditorButtonBevel(frame, r, false)
}

func drawEditorButtonBevel(frame []byte, r [4]int, depressed bool) {
	//05bc toggles AREA+8 first. The exit controller sets its statesFF at
	//1abf:2598..25a0, so its initial calls paint state0 (raised). Selected
	//category controls instead reach state1. The outer729e dialog has its
	//own opposite border and must not be changed with the button state.
	top, bottom := byte(15), byte(8)
	if depressed {
		top, bottom = bottom, top
	}
	x, y, w, h := r[0], r[1], r[2], r[3]
	_ = assetdata.OfficeLine(frame, x, y, x, y+h-1, top)
	_ = assetdata.OfficeLine(frame, x, y, x+w-1, y, top)
	_ = assetdata.OfficeLine(frame, x+1, y+h, x+w, y+h, bottom)
	_ = assetdata.OfficeLine(frame, x+w, y+1, x+w, y+h, bottom)
}

func (c *FrontEnd) drawHeldEditorIcon(frame []byte) {
	e, a := c.Editor, c.editorAssets()
	if e == nil || a == nil || !e.Held || e.Stage != "edit" || len(frame) != 64000 || !c.pointerInside {
		return
	}
	index := e.Category - 1
	switch e.Role {
	case "base":
		index = 10
	case "FARP":
		index = 11
	case "primary":
		index = 12
	case "secondary":
		index = 13
	}
	if index < 0 || index >= len(a.IconRects) {
		return
	}
	r, h := a.IconRects[index], a.IconAnchors[index]
	for y := 0; y < r[3]; y++ {
		for x := 0; x < r[2]; x++ {
			dx, dy := e.CursorX+x-h[0], e.CursorY+y-h[1]
			if dx < 0 || dx >= 320 || dy < 0 || dy >= 200 {
				continue
			}
			v := a.Icons[(r[1]+y)*320+r[0]+x]
			if v != 0 {
				frame[dy*320+dx] = v
			}
		}
	}
}
