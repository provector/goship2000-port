package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/world"
)

// GS SHA25646ce1a52...90e9, section1 static20f4; file=287c0+IP.
// C020 is independent of E289. BC8E/92 are reference divisors and BC96/9A
// are CURRENT binary32 camera offsets. The overview uses separate doubles.
type editorMapView struct {
	Detail                                 bool
	Bounds                                 [4]int // C364/C366/C378/C37A; inclusive input endpoints
	Reference                              [2]int // C354/C356
	ReferenceScaleX, ReferenceScaleY       float32
	CameraX, CameraY                       float32
	drag                                   string
	lastX, lastY, offsetX, offsetY, panSum int
	autoEntered                            bool // C022: an inner-rectangle visit admits subsequent scrolling
}

func newEditorMapView(layout int) editorMapView {
	v := editorMapView{ReferenceScaleX: 17.066667556762695, ReferenceScaleY: 17.454545974731445, CameraX: 688, CameraY: 512}
	v.layout(layout)
	return v
}
func (v *editorMapView) layout(layout int) {
	//20f4:0a9c..0b7c and page setup70fc; E289 is masked, not clamped.
	v.Bounds = [4][4]int{{0, 0, 320, 200}, {170, 65, 147, 123}, {170, 3, 147, 194}, {3, 3, 160, 128}}[layout&3]
	v.Reference = [4][2]int{{5, 130}, {231, 0}, {231, 0}, {171, 65}}[layout&3]
	v.release()
	v.clamp()
}
func (v *editorMapView) release() { v.drag = ""; v.panSum = 0; v.autoEntered = false }
func (v *editorMapView) clamp() {
	//0b7e: min against upper then max against lower; operands are floats.
	r := v.Bounds
	v.CameraX = max(float32(-r[0]), min(v.CameraX, float32(1536-r[2]-r[0])))
	v.CameraY = max(float32(r[1]+r[3]-200), min(v.CameraY, float32(r[1]+952)))
}
func (v *editorMapView) referenceBounds() [4]int {
	return [4]int{v.Reference[0], v.Reference[1], 89, 65}
}
func (v *editorMapView) point(x, y int) (int, int) {
	//0004/002a truncate the float top-left BEFORE adding the cursor delta.
	r := v.Bounds
	return int(float32(r[0])+v.CameraX) + x - r[0], int(float32(float32(r[1])-v.CameraY)+float32(952)) + y - r[1]
}
func (v *editorMapView) screen(x, y int) (int, int) {
	ox, oy := v.point(v.Bounds[0], v.Bounds[1])
	return x - ox + v.Bounds[0], y - oy + v.Bounds[1]
}
func (v *editorMapView) extent() [4]int {
	x, y := v.point(v.Bounds[0], v.Bounds[1])
	return [4]int{v.Reference[0] + int(float32(x)/v.ReferenceScaleX), v.Reference[1] + int(float32(y)/v.ReferenceScaleY), int(float32(v.Bounds[2]) / v.ReferenceScaleX), int(float32(v.Bounds[3]) / v.ReferenceScaleY)}
}
func (c *FrontEnd) editorMapBounds() [4]int {
	if c.Editor.View.Detail && !c.Editor.paintOverview {
		return c.Editor.View.Bounds
	}
	return [4]int{3, 3, 159, 127}
}
func (c *FrontEnd) editorPoint() (int, int) {
	e := c.Editor
	if e.View.Detail {
		return e.View.point(e.CursorX, e.CursorY)
	}
	return editorOverviewPoint(e.CursorX, e.CursorY)
}
func (c *FrontEnd) editorScreen(x, y int) (int, int) {
	if c.Editor.View.Detail && !c.Editor.paintOverview {
		return c.Editor.View.screen(x, y)
	}
	return editorOverviewScreen(x, y)
}
func (c *FrontEnd) editorMoveCursor(a Actions, pointerActivity bool) {
	e := c.Editor
	if pointerActivity {
		e.CursorX, e.CursorY = a.X, a.Y
	}
	if a.Left {
		e.CursorX--
	}
	if a.Right {
		e.CursorX++
	}
	if a.Up {
		e.CursorY--
	}
	if a.Down {
		e.CursorY++
	}
	e.CursorX = max(0, min(319, e.CursorX))
	e.CursorY = max(0, min(199, e.CursorY))
}
func (v *editorMapView) recenter(x, y int) {
	//10f2: truncate reference->detail products, then subtract half viewport.
	r := v.Bounds
	v.CameraX = float32(int(float32(x-v.Reference[0])*v.ReferenceScaleX) - r[2]/2 - r[0])
	v.CameraY = float32(r[3]/2 - int(float32(y-v.Reference[1])*v.ReferenceScaleY) + r[1] + 952)
	v.clamp()
}
func (c *FrontEnd) handleEditorMapInput(a Actions, select1, select2 bool) (bool, error) {
	e := c.Editor
	v := &e.View
	if !v.Detail {
		return false, nil
	}
	x, y := e.CursorX, e.CursorY
	if v.drag != "" {
		if a.PointerReset || a.FlightFocusLost || !a.Inside {
			v.release()
			return true, nil
		}
		active := a.PointerLeftHeld
		if v.drag != "reference" {
			active = a.PointerRightHeld
		}
		if a.PointerStateValid && (!active || a.PointerReleased) {
			pick := v.drag == "pan" && v.panSum == 0
			v.release()
			if pick {
				return true, c.editorPickUp()
			}
			return true, nil
		}
		if a.PointerMoved || a.Left || a.Right || a.Up || a.Down {
			switch v.drag {
			case "reference":
				//0da2: original logical pointer limits321-89/201-65.
				x = max(v.offsetX, min(v.offsetX+232, x))
				y = max(v.offsetY, min(v.offsetY+136, y))
				e.CursorX, e.CursorY = x, y
				v.Reference = [2]int{x - v.offsetX, y - v.offsetY}
			case "recenter":
				v.recenter(x, y)
			case "pan":
				dx, dy := v.lastX-x, y-v.lastY
				v.panSum += dx + dy
				v.CameraX = float32(v.CameraX + float32(dx))
				v.CameraY = float32(v.CameraY + float32(dy))
				v.clamp()
			}
			v.lastX, v.lastY = x, y
			c.updateEditorShadow()
		}
		return true, nil
	}
	if (select1 || select2) && (!a.PointerPressed || a.Inside) && editorInclusive(x, y, v.referenceBounds()) {
		if select1 {
			if a.PointerPressed && a.PointerStateValid {
				v.drag = "reference"
				v.offsetX = x - v.Reference[0]
				v.offsetY = y - v.Reference[1]
			}
		} else {
			v.recenter(x, y)
			if a.PointerPressed && a.PointerStateValid {
				v.drag = "recenter"
			}
		}
		v.lastX, v.lastY = x, y
		c.updateEditorShadow()
		return true, nil
	}
	if select2 && editorInclusive(x, y, v.Bounds) && a.PointerPressed && a.Inside && a.PointerStateValid {
		v.drag = "pan"
		v.lastX, v.lastY = x, y
		v.panSum = 0
		return true, nil
	}
	if e.Held && e.AutoScroll {
		//78b4..7a19: source inset24x18, equality stays inside; one axis
		// per input iteration, and the pointer is returned to that inset.
		r := v.Bounds
		inner := [4]int{r[0] + 24, r[1] + 18, r[2] - 48, r[3] - 36}
		if editorInclusive(x, y, inner) {
			v.autoEntered = true
		} else if v.autoEntered {
			switch {
			case x < inner[0]:
				v.CameraX = float32(v.CameraX + float32(x-inner[0]))
				e.CursorX = inner[0]
			case y < inner[1]:
				v.CameraY = float32(v.CameraY - float32(y-inner[1]))
				e.CursorY = inner[1]
			case x > inner[0]+inner[2]:
				v.CameraX = float32(v.CameraX + float32(x-inner[0]-inner[2]))
				e.CursorX = inner[0] + inner[2]
			case y > inner[1]+inner[3]:
				v.CameraY = float32(v.CameraY - float32(y-inner[1]-inner[3]))
				e.CursorY = inner[1] + inner[3]
			}
			v.clamp()
			c.updateEditorShadow()
		}
	}
	return false, nil
}

// The same prepared64x64 grid and192x144 atlas consumed by navigation.GPS
// draw source24x18 map tiles. MIS is only the overview artwork.
func drawEditorDetailTerrain(frame []byte, w *world.World, v *editorMapView) {
	if w == nil || len(w.Grid) != 4096 || len(w.TileAtlas) != 192*144 {
		return
	}
	r := v.Bounds
	for y := max(0, r[1]); y <= min(199, r[1]+r[3]); y++ {
		for x := max(0, r[0]); x <= min(319, r[0]+r[2]); x++ {
			wx, wy := v.point(x, y)
			if wx < 0 || wx >= 1536 || wy < 0 || wy >= 1152 {
				frame[y*320+x] = 0
				continue
			}
			id := int(w.Grid[(wy/18)*64+wx/24] & 63)
			frame[y*320+x] = w.TileAtlas[((id>>3)*18+wy%18)*192+(id&7)*24+wx%24]
		}
	}
}
func (c *FrontEnd) drawEditorDetail(frame []byte) {
	e := c.Editor
	v := &e.View
	if !v.Detail || c.Office.Assets.World == nil {
		return
	}
	w := c.Office.Assets.World.Worlds[editorMapKey(e.Document.World)]
	drawEditorDetailTerrain(frame, w, v)
}
func (c *FrontEnd) drawEditorReference(frame []byte) {
	e := c.Editor
	v := &e.View
	if !v.Detail || c.Office.Assets.World == nil {
		return
	}
	w := c.Office.Assets.World.Worlds[editorMapKey(e.Document.World)]
	if w == nil || len(w.Overview) != 89*65 {
		return
	}
	for y := 0; y < 65; y++ {
		for x := 0; x < 89; x++ {
			dx, dy := x+v.Reference[0], y+v.Reference[1]
			if dx >= 0 && dx < 320 && dy >= 0 && dy < 200 {
				frame[dy*320+dx] = w.Overview[y*89+x]
			}
		}
	}
	r := v.extent()
	for _, line := range [][4]int{{r[0], r[1], r[0] + r[2], r[1]}, {r[0], r[1] + r[3], r[0] + r[2], r[1] + r[3]}, {r[0], r[1], r[0], r[1] + r[3]}, {r[0] + r[2], r[1], r[0] + r[2], r[1] + r[3]}} {
		_ = assetdata.OfficeLine(frame, line[0], line[1], line[2], line[3], 14)
	}
	for _, p := range e.Document.Placements {
		if p.Role == "defender" {
			continue
		}
		x := v.Reference[0] + int(float64(p.X)*0.057937427578215524)
		y := v.Reference[1] + int(float64(p.Y)*0.05643340857787811)
		assetdata.OfficeFill(frame, x, y, 2, 2, 15)
	}
}
