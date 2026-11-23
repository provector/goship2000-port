package menus

import gamestate "gunship2000-assets/game/state"

// Source20f4:2114's ordinary map callback chooses an existing atlas row, not
// a new glyph color. Command category+1 selectsY+16; category9 additionally
// selectsY+4 if any actually consumed component has weaponbyte0. Historical
// descriptors keep their recorded component expansion through AuthoredComponents.
// Base/FARP/goal marker callers have separate source owners and stay unchanged.
func (c *FrontEnd) editorOrdinaryIconRow(p gamestate.AuthoredPlacement, index int) (int, bool) {
	if index < 0 {
		return 0, false
	}
	if p.Role != "defender" || index >= 10 {
		return 0, true
	}
	row := 0
	if c.Editor.Role == "defender" && c.Editor.Category == index+1 {
		row = 16
	}
	if index != 9 {
		return row, true
	}
	w, err := gamestate.AuthoredWorld(c.editorData(), c.Editor.Document.World)
	if err != nil {
		return 0, false
	}
	rows, err := gamestate.AuthoredComponents(c.editorData(), c.Editor.Document, p)
	if err != nil {
		return 0, false
	}
	for _, ci := range rows {
		if ci < 0 || ci >= len(w.FRMD)/8 {
			return 0, false
		}
		if w.FRMD[ci*8+3] == 0 {
			row += 4
			break
		}
	}
	return row, true
}

// Source20F4:2223..2307 repeats the category glyph at (-4,+5) and
// (+4,+5) when FRMD+2==1. These are source screen-space formation marks,
// not fabricated world positions for every formation component.
func (c *FrontEnd) editorFormationIcon(frame []byte, p gamestate.AuthoredPlacement, index, x, y, row int) {
	if index == 10 || index == 11 {
		return
	}
	w, err := gamestate.AuthoredEditorWorld(c.editorData(), c.Editor.Document.World)
	if err != nil || p.Component < 0 || p.Component >= len(w.FRMD)/8 || w.FRMD[p.Component*8+2] != 1 {
		return
	}
	if index == 9 {
		rows, err := gamestate.AuthoredComponents(c.editorData(), c.Editor.Document, p)
		if err != nil {
			return
		}
		for _, ci := range rows {
			if w.FRMD[ci*8+3] != 0 {
				row &= ^4
				break
			}
		}
	}
	c.editorIconRow(frame, index, x-4, y+5, false, row)
	c.editorIconRow(frame, index, x+4, y+5, false, row)
}
