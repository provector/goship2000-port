package cockpit

import "fmt"

// MapRoute is a read-only projection of one DATA272d:0488+70*n row. Mode,
// index and formation group remain separate source domains; no renderer Side
// label determines their visibility or colour.
type MapRoute struct {
	Index  int8
	Mode   byte
	Group  int8
	H0, H1 uint32
	Points [3]MapPoint
}

type MapPoint struct{ H0, H1 uint32 }

func (a *Assets) drawMapRoutes(dst []byte, in MapInputs, x, y int, clip Rect) {
	project := func(p MapPoint) (int, int) {
		// Source signed long division followed by low-word add. The vertical
		// divisor is negative, independently of the horizontal341.
		return int(int16(int16(x) + int16(int32(p.H0-in.H0)/341))), int(int16(int16(y) + int16(int32(p.H1-in.H1)/-455)))
	}
	for slot, r := range in.Routes {
		if r.Index == 0 || r.Mode == 0 {
			continue
		}
		ink := byte(7)
		// 015b:08dd..0930/b10..b34 compares signed group bytes with the
		// current runtime row. The ordinary player panel passes selected=-1,
		// so the GPS selected-row flashing bit is deliberately absent here.
		if int16(in.Routes[4].Group) == int16(in.Observer) ||
			(int16(in.Routes[3].Group) == int16(in.Observer) && slot < 3) ||
			(int16(in.Routes[3].Group) != int16(in.Observer) && slot >= 2) {
			ink = 1
		}
		line := func(from, to MapPoint) {
			x0, y0 := project(from)
			x1, y1 := project(to)
			Line(dst, x0, y0, x1, y1, ink, clip)
		}
		if r.Points[1].H0 != 0 {
			line(r.Points[0], r.Points[1])
		}
		if r.Points[2].H0 != 0 {
			line(r.Points[1], r.Points[2])
		}
		if r.Index == 1 {
			line(MapPoint{r.H0, r.H1}, r.Points[0])
		}
		for n, point := range r.Points {
			if point.H0 == 0 {
				break // Original0bdf tests H0 only, then stops this route's symbols.
			}
			sx, sy := project(point)
			shift := uint(8)
			if int(r.Index) == n+1 && (in.RoutePhase&4) == 0 {
				shift++
			}
			a.DrawVectors(dst, a.MapVectors[[3]string{"map-078c", "map-0773", "map-073d"}[n]], sx, sy, 0, shift, 0, clip)
		}
	}
}

// remapMFD is the selected-VGA19a1->0e34 destination operation. It maps the
// already composed display through the source brightness table into bank15h;
// it never changes the scene palette. E1.bit0 alternate panels retain colour.
func (a *Assets) remapMFD(dst []byte, d Definition, secondary bool, flags byte) error {
	if flags&1 != 0 || (secondary && d.Parameters[0x52/2] != d.Parameters[0x5e/2]) {
		return nil
	}
	if len(a.ScenePresentation.ConfirmationRemap) != 256 {
		return fmt.Errorf("MFD requires selected-VGA brightness remap")
	}
	base := 0x54
	if secondary {
		base = 0x60
	}
	x0, y0 := int(d.Parameters[base/2]), int(d.Parameters[(base+2)/2])
	x1, y1 := int(d.Parameters[(base+4)/2]), int(d.Parameters[(base+6)/2])
	if x0 < 0 || y0 < 0 || x1 >= 320 || y1 >= 200 || x0 > x1 || y0 > y1 {
		return fmt.Errorf("MFD brightness bounds outside indexed target")
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			at := y*320 + x
			PutLogical(dst, at, a.ScenePresentation.ConfirmationRemap[dst[at]]+0x15)
		}
	}
	return nil
}

// DrawMastFrame follows section2:2242..226f, reached after BOTH the target
// panel and the empty-panel fill. Sprite1929 flags4 preserve only index00.
// Nonzero source pixels inside the rectangular preview are the sight bezel.
func (a *Assets) DrawMastFrame(dst []byte) error {
	return a.Sprite(dst, "mast-view", 55, 127, Full)
}

func (a *Assets) DrawMastFrameScaled(dst []byte, scale int) error {
	return LogicalPass(dst, scale, a.DrawMastFrame)
}
