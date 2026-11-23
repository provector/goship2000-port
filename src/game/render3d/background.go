package render3d

import "fmt"

// background follows2414's indexed nested quads. It deliberately retains pixels
// outside its sky polygons; the source gradient branch is not a universal clear.
func (f *frame) background() error {
	logicalView := f.scene.view()
	a, c := f.scene.Camera.Angles[0], f.scene.Camera.Angles[2]
	sa, ca := Sine(a, f.r.Assets.Sine), Sine(a+0x4000, f.r.Assets.Sine)
	sc, cc := Sine(c, f.r.Assets.Sine), Sine(c+0x4000, f.r.Assets.Sine)
	h := -int16(f.scene.Camera.Height << 1)
	x := qmul(-qmul(ca, sc), h) + qmul(sa, sc)
	y := qmul(qmul(ca, cc), h) - qmul(sa, cc)
	den := qmul(sa, h) + ca
	divide := func(v int16) (int, error) { return idiv(int64(v)<<8, int64(den)) }
	x0, e := divide((x >> 1) - cc)
	if e != nil {
		return e
	}
	x1, e := divide((x >> 1) + cc)
	if e != nil {
		return e
	}
	y0, e := divide((y >> 1) - sc)
	if e != nil {
		return e
	}
	y1, e := divide((y >> 1) + sc)
	if e != nil {
		return e
	}
	a0 := Point{int(int16(logicalView.Center.X + x0)), int(int16(logicalView.Center.Y - y0))}
	b := Point{int(int16(logicalView.Center.X + x1)), int(int16(logicalView.Center.Y - y1))}
	c0 := Point{int(int16(b.X + b.Y - a0.Y)), int(int16(b.Y + a0.X - b.X))}
	d := Point{int(int16(a0.X + b.Y - a0.Y)), int(int16(a0.Y + a0.X - b.X))}
	mid := func(a, b Point) Point { return Point{int(int16(a.X+b.X) >> 1), int(int16(a.Y+b.Y) >> 1)} }
	// Keep the legacy subdivision count and material words. At larger sizes
	// carry subpixel horizon coordinates through subdivision to screen clipping.
	layout := f.scene.Layout()
	var qa, qb, qc, qd Point
	if layout.Mapped() {
		frac := func(v int16, rounded int) int { return int((int64(v)<<16)/int64(den)) - rounded*256 }
		qa = Point{a0.X*256 + frac((x>>1)-cc, x0), a0.Y*256 - frac((y>>1)-sc, y0)}
		qb = Point{b.X*256 + frac((x>>1)+cc, x1), b.Y*256 - frac((y>>1)+sc, y1)}
		qc = Point{qb.X + qb.Y - qa.Y, qb.Y + qa.X - qb.X}
		qd = Point{qa.X + qb.Y - qa.Y, qa.Y + qa.X - qb.X}
	}
	lo, hi := byte(0xa1), byte(0xa0)
	for step := 0; step < 32; step++ {
		left, right := mid(a0, d), mid(b, c0)
		lo++
		hi++
		if lo&15 == 0 {
			lo--
			hi--
		}
		points := []Point{left, right, c0, d}
		if layout.Mapped() {
			qleft, qright := Point{(qa.X + qd.X) >> 1, (qa.Y + qd.Y) >> 1}, Point{(qb.X + qc.X) >> 1, (qb.Y + qc.Y) >> 1}
			for i, q := range [...]Point{qleft, qright, qc, qd} {
				points[i] = Point{q.X * layout.Width / (256 * Width), layout.Top + q.Y*layout.PictureHeight/(256*Height)}
			}
			qc, qd = qright, qleft
		} else if l := layout; l.Width != Width {
			for i := range points {
				points[i].X = points[i].X * l.Width / Width
				points[i].Y = l.Top + points[i].Y*l.PictureHeight/Height
			}
		}
		_, err := FillPolygon(f.dst, points, uint16(lo)|uint16(hi)<<8, f.view.Bounds, f.scene.GraphicsOptions, nil)
		if err != nil {
			return err
		}
		if f.scene.SkyMask != nil {
			if _, err = FillPolygon(f.scene.SkyMask, points, uint16(hi)*0x101, f.view.Bounds, f.scene.GraphicsOptions&^1, nil); err != nil {
				return err
			}
		}
		c0, d = right, left
		spanY := uint16(int16(int(int16(c0.Y+d.Y)>>1)+1-int(int16(a0.Y+b.Y)>>1))) >> 1
		spanX := uint16(int16(int(int16(c0.X+d.X)>>1)+1-int(int16(a0.X+b.X)>>1))) >> 1
		if spanY == 0 && spanX == 0 {
			return nil
		}
	}
	return fmt.Errorf("source sky subdivision did not converge in32 steps")
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
