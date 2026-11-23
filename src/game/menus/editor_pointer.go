package menus

// GS20f4:294c chooses one sourceMBICONS bank2 cursor;686 owns the crop
// dimensions/hotspots. These extents are counts (1196 copies cx×cy), not the
// inclusive hit rectangles. The editor owns its full200-line canvas.
var editorPointerAtlas = [...][6]int{
	{0, 26, 13, 16, 7, 5}, {25, 15, 11, 9, 6, 6}, {39, 0, 11, 11, 4, 1},
	{25, 6, 11, 9, 4, 1}, {25, 0, 14, 6, 7, 2}, {14, 26, 12, 16, 6, 10}, {39, 11, 6, 11, 1, 0},
}

func (c *FrontEnd) editorPointerStyle() int {
	e := c.Editor
	if e == nil {
		return 6
	}
	if e.Stage != "edit" {
		if e.Stage == "numeric" {
			if e.Numeric == "wind" && e.NumericColumn == 1 {
				return 3
			}
			return 2
		}
		return 6
	}
	if e.View.Detail {
		if editorInclusive(e.CursorX, e.CursorY, e.View.referenceBounds()) {
			return 4
		}
		if editorInclusive(e.CursorX, e.CursorY, e.View.Bounds) {
			if e.View.drag != "" {
				return 1
			}
			if e.Held {
				return 0
			}
			return 6
		}
	}
	if e.Held && editorInclusive(e.CursorX, e.CursorY, [4]int{3, 3, 159, 127}) {
		return 5
	}
	// 2a9e is the map-control strip, not the lower-right EXIT control.
	if editorInclusive(e.CursorX, e.CursorY, [4]int{2, 130, 161, 10}) {
		return 4
	}
	return 6
}
func (c *FrontEnd) drawEditorPointer(frame []byte) {
	e, a := c.Editor, c.editorAssets()
	if e == nil || a == nil || !c.pointerInside || len(frame) != 64000 || len(a.Icons) != 64000 {
		return
	}
	r := editorPointerAtlas[c.editorPointerStyle()]
	for y := 0; y < r[3]; y++ {
		for x := 0; x < r[2]; x++ {
			dx, dy := e.CursorX+x-r[4], e.CursorY+y-r[5]
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
