package menus

import gamestate "gunship2000-assets/game/state"

// GS46ce...90e9, initialized DS2B9D+10*category, read20F4:5462.
// The fixed overview uses one-pixel anchors. These are not the MBICONS atlas
// silhouettes drawn by the independent detail callback20F4:2114.
var editorOverviewCategoryInk = [...]byte{11, 68, 68, 68, 68, 11, 68, 68, 68, 6}

func (c *FrontEnd) editorSymbolPut(frame []byte, x, y int, ink byte) {
	b := c.editorMapBounds()
	if len(frame) == 64000 && x >= 0 && x < 320 && y >= 0 && y < 200 && editorInclusive(x, y, b) {
		frame[y*320+x] = ink
	}
}

func (c *FrontEnd) editorOverviewOrdinary(frame []byte, p gamestate.AuthoredPlacement, cat, x, y int) {
	if cat < 0 || cat >= len(editorOverviewCategoryInk) || !c.editorCategoryVisible(cat) {
		return
	}
	ink := editorOverviewCategoryInk[cat]
	if c.Editor.Role == "defender" && c.Editor.Category == cat+1 {
		ink = 14
	}
	rows, err := gamestate.AuthoredComponents(c.editorData(), c.Editor.Document, p)
	if err != nil {
		return
	}
	w, err := gamestate.AuthoredEditorWorld(c.editorData(), c.Editor.Document.World)
	if err != nil || p.Component < 0 || p.Component*8+3 >= len(w.FRMD) {
		return
	}
	//540A:5619..5650 sets structures' anchor6 if any member is unarmed.
	//Its two formation flank pixels instead use44 if any member is armed.
	flank := ink
	if cat == 9 {
		ink, flank = 0x44, 6
		for _, ci := range rows {
			if ci < 0 || ci*8+3 >= len(w.FRMD) {
				return
			}
			if w.FRMD[ci*8+3] == 0 {
				ink = 6
			} else {
				flank = 0x44
			}
		}
	}
	c.editorSymbolPut(frame, x, y, ink)
	//20F4:566E..56DD: type1 alone draws (-1,+1),(+1,+1).
	if w.FRMD[p.Component*8+2] == 1 {
		c.editorSymbolPut(frame, x-1, y+1, flank)
		c.editorSymbolPut(frame, x+1, y+1, flank)
	}
}

func (c *FrontEnd) editorSearchSymbol(frame []byte, role string, x, y int) {
	//DS2B80/2B8C, source5C83/5D4A and detail2545/25C1.
	sx, row := 140, 0
	if role == "secondary" {
		sx = 148
	}
	if c.Editor.SearchRole == role {
		row = 16
	}
	a := c.editorAssets()
	if len(a.Icons) != 64000 {
		return
	}
	for yy := 0; yy < 7; yy++ {
		for xx := 0; xx < 7; xx++ {
			if ink := a.Icons[(row+yy)*320+sx+xx]; ink != 0 {
				c.editorSymbolPut(frame, x+xx-3, y+yy-3, ink)
			}
		}
	}
}

// Deep Strike is an edge bar plus the normal base glyph, not an invented base
// footprint. Overview5A35 uses(-12,+20) and a+4 orthogonal bias; detail239B
// uses initialized DS2D7C=40 with source rectangle admission/clamping.
func (c *FrontEnd) editorEntrySymbol(frame []byte, p gamestate.AuthoredPlacement, overview bool) {
	if p.Entry == nil || p.Role != "base" {
		return
	}
	x, y := c.editorScreen(p.X, p.Y)
	horizontal := p.Y < 50 || p.Y > 1105
	if overview {
		if horizontal {
			c.editorClippedControlLine(frame, x-12, y+4, x+20, y+4, 1)
		} else {
			c.editorClippedControlLine(frame, x+4, y-12, x+4, y+20, 1)
		}
		return
	}
	b := c.editorMapBounds()
	if horizontal {
		if x < b[0]-40 || x > b[0]+b[2]+39 || y < b[1] || y > b[1]+b[3]-1 {
			return
		}
		y = max(b[1], min(b[1]+b[3]-1, y))
		c.editorClippedControlLine(frame, max(b[0], min(b[0]+b[2]-1, x-40)), y, max(b[0], min(b[0]+b[2]-1, x+40)), y, 1)
	} else {
		if x < b[0] || x > b[0]+b[2]-1 || y < b[1]-40 || y > b[1]+b[3]+39 {
			return
		}
		x = max(b[0], min(b[0]+b[2]-1, x))
		c.editorClippedControlLine(frame, x, max(b[1], min(b[1]+b[3]-1, y-40)), x, max(b[1], min(b[1]+b[3]-1, y+40)), 1)
	}
}

func (c *FrontEnd) drawEditorPlacedSymbols(frame []byte, overview bool) {
	e := c.Editor
	//Source overview visits ordinary descriptors ascending; detail230E visits
	//descending. Special role records are separate callers, so shared targets
	//are painted as their ordinary carrier first and objective marker later.
	draw := func(p gamestate.AuthoredPlacement) {
		index := c.editorIconCategory(p)
		switch p.Role {
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
		c.editorEntrySymbol(frame, p, overview)
		if overview && p.Role == "defender" {
			c.editorOverviewOrdinary(frame, p, index, x, y)
		} else if row, ok := c.editorOrdinaryIconRow(p, index); ok {
			c.editorIconRow(frame, index, x, y, false, row)
			//The fixed overview's P/S callers5C4F..5DB8 draw one
			//site marker. Detail2519..253F deliberately calls2114,
			//whose type1 formation flanks also apply to goal records.
			if !overview {
				c.editorFormationIcon(frame, p, index, x, y, row)
			}
		}
		//A search/destination marker is a second record. It must never move
		//the actual unit/goal glyph away from its physical descriptor.
		if p.Search != nil {
			x, y = c.editorScreen(p.Search.X, p.Search.Y)
			c.editorSearchSymbol(frame, p.Role, x, y)
		} else if p.Route != nil {
			x, y = c.editorScreen(p.Route.X, p.Route.Y)
			c.editorSearchSymbol(frame, p.Role, x, y)
		}
	}
	if overview {
		for _, p := range e.Document.Placements {
			if p.Role == "defender" {
				draw(p)
			}
		}
	} else {
		for i := len(e.Document.Placements) - 1; i >= 0; i-- {
			if p := e.Document.Placements[i]; p.Role == "defender" {
				draw(p)
			}
		}
	}
	for _, role := range []string{"base", "FARP", "primary", "secondary"} {
		for _, p := range e.Document.Placements {
			if p.Role == role {
				draw(p)
			}
		}
	}
}
