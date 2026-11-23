package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
)

// SourceMemoryFallbackPolicy is a visible approximation when original draw-time
// CS/DS bytes are unavailable. The source feedback leaf can emit only the four
// indices in DL's low-two-bit family; it cannot emit arbitrary black/white.
const SourceMemoryFallbackPolicy = "source-index-family-2bit-hatch-v1"

// VGA0515..052b reads one byte from each carrier at the first destination
// of each pair, after an optional plain leading byte. A one-pixel span needs
// no memory witness at all. Never admit zero-padded holes in a partial carrier.
func (c *Carrier) supportsSpan(y, left, right int) bool {
	first := left + ((right - left + 1) & 1)
	if first > right {
		return true
	}
	if c == nil {
		return false
	}
	for at, end := y*Width+first, y*Width+right; at <= end; at += 2 {
		if at < 0 || at >= len(c.CS) || at >= len(c.DS) {
			return false
		}
		if c.Readable != nil {
			known := false
			for _, r := range c.Readable {
				if at >= r.First && at < r.End {
					known = true
					break
				}
			}
			if !known {
				return false
			}
		}
	}
	return true
}

// ClipLine retains original endpoints for every quotient. Reusing rounded
// intersections changes corner coverage (037f:003c).
func ClipLine(a, b Point, bounds Rect) (Point, Point, bool, int, error) {
	return clipLineTarget(a, b, bounds, false)
}

// Physical scale2 coordinates must not enter the source signed-word overflow
// sites: doubling an otherwise valid edge would saturate its delta and bend it.
// Keep source clipping order/closure while widening only this display domain.
func clipLineTarget(a, b Point, bounds Rect, wide bool) (Point, Point, bool, int, error) {
	origA, origB := a, b
	lastX := -1 // Source SI survives clipping: CB/155 write it;8B/99 return it.
	for tries := 0; tries < 16; tries++ {
		if a.X < bounds.Left && b.X < bounds.Left {
			return a, b, false, bounds.Left, nil
		}
		if a.X > bounds.Right && b.X > bounds.Right {
			return a, b, false, bounds.Right, nil
		}
		if a.Y < bounds.Top && b.Y < bounds.Top || a.Y > bounds.Bottom && b.Y > bounds.Bottom {
			// Polygon2065 still closes this X boundary after vertical rejection.
			return a, b, false, lastX, nil
		}
		changed := false
		for i := 0; i < 2; i++ {
			p := a
			if i == 1 {
				p = b
			}
			var n int64
			var d int
			divide := func() (int, error) {
				if wide {
					if d == 0 {
						return 0, fmt.Errorf("screen clipping division by zero")
					}
					return int(n / int64(d)), nil
				}
				return idiv(n, int64(d))
			}
			if p.X < bounds.Left || p.X > bounds.Right {
				if p.X < bounds.Left {
					p.X = bounds.Left
				} else {
					p.X = bounds.Right
				}
				lastX = p.X
				n = int64(int16(origB.Y-origA.Y)) * int64(intoWord(p.X-origA.X))
				d = intoWord(origB.X - origA.X)
				if wide {
					n = int64(origB.Y-origA.Y) * int64(p.X-origA.X)
					d = origB.X - origA.X
				}
				q, err := divide()
				if err != nil {
					return a, b, false, -1, err
				}
				p.Y = int(int16(origA.Y + q))
				if wide {
					p.Y = origA.Y + q
				}
			} else if p.Y < bounds.Top || p.Y > bounds.Bottom {
				p.Y = bounds.Top
				if (i == 0 && a.Y > bounds.Bottom) || (i == 1 && b.Y > bounds.Bottom) {
					p.Y = bounds.Bottom
				}
				n = int64(intoWord(origB.X-origA.X)) * int64(intoWord(p.Y-origA.Y))
				d = int(int16(origB.Y - origA.Y))
				if wide {
					n = int64(origB.X-origA.X) * int64(p.Y-origA.Y)
					d = origB.Y - origA.Y
				}
				q, err := divide()
				if err != nil {
					return a, b, false, -1, err
				}
				p.X = int(int16(origA.X + q))
				if wide {
					p.X = origA.X + q
				}
			} else {
				continue
			}
			if _, err := wordChecked(p.X); err != nil && !wide {
				return a, b, false, -1, err
			}
			if _, err := wordChecked(p.Y); err != nil && !wide {
				return a, b, false, -1, err
			}
			if i == 0 {
				a = p
			} else {
				b = p
			}
			changed = true
			break
		}
		if !changed {
			return a, b, true, -1, nil
		}
	}
	return a, b, false, -1, fmt.Errorf("source clip did not converge")
}

func boundsValid(b Rect) bool {
	return b.Left >= 0 && b.Top >= 0 && b.Right < Width && b.Bottom < Height && b.Left <= b.Right && b.Top <= b.Bottom
}

// visitLine uses the source half-major initial error and strict borrow tie.
func visitLine(a, b Point, visit func(int, int)) {
	dx, dy := b.X-a.X, b.Y-a.Y
	sx, sy := 1, 1
	if dx < 0 {
		sx = -1
		dx = -dx
	}
	if dy < 0 {
		sy = -1
		dy = -dy
	}
	x, y := a.X, a.Y
	if dx >= dy {
		err := dx / 2
		for i := 0; i <= dx; i++ {
			visit(x, y)
			x += sx
			err -= dy
			if err < 0 {
				err += dx
				y += sy
			}
		}
	} else {
		err := dy / 2
		for i := 0; i <= dy; i++ {
			visit(x, y)
			y += sy
			err -= dx
			if err < 0 {
				err += dy
				x += sx
			}
		}
	}
}

func DrawLine(dst []byte, a, b Point, ink byte, bounds Rect) error {
	if !targetBoundsValid(dst, bounds) {
		return fmt.Errorf("invalid indexed line target or bounds")
	}
	a, b, ok, _, err := clipLineTarget(a, b, bounds, targetWidth(dst) != Width)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	visitLine(a, b, func(x, y int) {
		if x >= bounds.Left && x <= bounds.Right && y >= bounds.Top && y <= bounds.Bottom {
			dst[y*targetWidth(dst)+x] = ink
		}
	})
	return nil
}

type spans struct{ left, right [1024]int }

// reset initializes all rows visible to this caller, including boundary closures.
func (s *spans) reset(top, bottom int) {
	for y := top; y <= bottom; y++ {
		s.left[y] = 32768
		s.right[y] = -32768
	}
}
func (s *spans) add(x, y int) {
	if y >= 0 && y < len(s.left) {
		if x < s.left[y] {
			s.left[y] = x
		}
		if x > s.right[y] {
			s.right[y] = x
		}
	}
}
func (s *spans) vertical(x, y0, y1 int, b Rect) {
	if y0 < b.Top {
		y0 = b.Top
	}
	if y0 > b.Bottom {
		y0 = b.Bottom
	}
	if y1 < b.Top {
		y1 = b.Top
	}
	if y1 > b.Bottom {
		y1 = b.Bottom
	}
	if y0 == y1 {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		s.add(x, y)
	}
}

// polygonSpans preserves 2065's vertical-boundary closure for rejected edges.
// Merely clipping surviving edges leaves holes in partially offscreen polygons.
type edgeClipper func(int, Point, Point, Rect) (Point, Point, bool, int, error)

// The destination belongs to this raster call, including during callbacks.
func (s *spans) polygon(points []Point, b Rect, clip edgeClipper) error {
	s.reset(b.Top, b.Bottom)
	if len(points) < 2 {
		return nil
	}
	for i, oa := range points {
		ob := points[(i+1)%len(points)]
		var a, z Point
		var ok bool
		var reason int
		var err error
		if clip == nil {
			a, z, ok, reason, err = ClipLine(oa, ob, b)
		} else {
			a, z, ok, reason, err = clip(i, oa, ob, b)
		}
		if err != nil {
			return err
		}
		if !ok {
			if reason >= 0 {
				s.vertical(reason, oa.Y, ob.Y, b)
			}
			continue
		}
		boundary := func(x int) bool { return x == b.Left || x == b.Right }
		if a.X != oa.X {
			if boundary(a.X) {
				s.vertical(a.X, a.Y, oa.Y, b)
			}
			if z.X != ob.X && boundary(z.X) {
				s.vertical(z.X, z.Y, ob.Y, b)
			}
		} else if z.X != ob.X && boundary(z.X) {
			// 20c5 reloads AX from the second clipped X before20ec tests
			// its boundary. Testing the unchanged first X leaves holes.
			s.vertical(z.X, z.Y, ob.Y, b)
		}
		// Source 2165 orders endpoints by Y before choosing its scan leaf.
		if a.Y > z.Y {
			a, z = z, a
		}
		visitLine(a, z, s.add)
	}
	return nil
}

// FillPolygon applies world-fill1960 semantics, including destination reads.
// Diagnostic is returned only for an unresolved source-memory material fallback.
func FillPolygon(dst []byte, points []Point, word uint16, bounds Rect, options byte, carrier *Carrier) (diagnostic bool, err error) {
	return fillPolygonWith(dst, points, word, bounds, options, carrier, nil)
}

func fillPolygonWith(dst []byte, points []Point, word uint16, bounds Rect, options byte, carrier *Carrier, clip edgeClipper) (diagnostic bool, err error) {
	return fillPolygonMaterial(dst, points, word, bounds, options, carrier, clip, nil)
}

func fillPolygonMaterial(dst []byte, points []Point, word uint16, bounds Rect, options byte, carrier *Carrier, clip edgeClipper, feedback *world.TerrainFeedback) (diagnostic bool, err error) {
	return fillPolygonMaterialOwned(dst, points, word, bounds, options, carrier, clip, feedback, nil)
}
func fillPolygonMaterialOwned(dst []byte, points []Point, word uint16, bounds Rect, options byte, carrier *Carrier, clip edgeClipper, feedback *world.TerrainFeedback, owner *spanOwnership) (diagnostic bool, err error) {
	return fillPolygonSpans(dst, points, word, bounds, options, carrier, clip, feedback, owner, 0)
}

func fillPolygonSpans(dst []byte, points []Point, word uint16, bounds Rect, options byte, carrier *Carrier, clip edgeClipper, feedback *world.TerrainFeedback, owner *spanOwnership, field SurfaceOwner) (diagnostic bool, err error) {
	if !targetBoundsValid(dst, bounds) {
		return false, fmt.Errorf("invalid indexed polygon target or bounds")
	}
	if clip == nil && targetWidth(dst) != Width {
		clip = func(_ int, a, z Point, b Rect) (Point, Point, bool, int, error) { return clipLineTarget(a, z, b, true) }
	}
	var s spans
	if err := s.polygon(points, bounds, clip); err != nil {
		return false, err
	}
	// Retain the accepted whole-primitive fallback admission, including its
	// no-read one-pixel edge spans, without repeating projection or clipping.
	if owner != nil && feedback == nil && word>>8 == 0 && word != 0 && options&1 != 0 {
		layout := rasterlayout.FromPixels(len(dst))
		for y := bounds.Top; y <= bounds.Bottom; y++ {
			l, r := max(s.left[y], bounds.Left), min(s.right[y], bounds.Right)
			if l <= r && s.left[y] < bounds.Right && s.right[y] > bounds.Left && !carrier.supportsSpan(layout.LogicalY[y], layout.LogicalX[l], layout.LogicalX[r]) {
				owner.fallback = true
				break
			}
		}
	}
	for y := bounds.Top; y <= bounds.Bottom; y++ {
		l, r := s.left[y], s.right[y]
		if l > r {
			continue
		}
		// Every selected-VGA world-fill leaf rejects a span lying wholly
		// on either clip boundary (040b:043c..0446 and the parallel leaves).
		// Accepted spans still include both endpoints; this is not a half-open fill.
		if l >= bounds.Right || r <= bounds.Left {
			continue
		}
		if l < bounds.Left {
			l = bounds.Left
		}
		if r > bounds.Right {
			r = bounds.Right
		}
		if l > r {
			continue
		}
		if field != 0 {
			width := targetWidth(dst)
			mappedFieldSpan(dst[y*width:], y, l, r, rasterlayout.FromPixels(len(dst)), field, feedback)
			continue
		}
		owner.span(dst, y, l, r, word)
		fallback := false
		if feedback != nil {
			fillTerrainFeedbackSpan(dst, y, l, r, byte(word), feedback)
		} else {
			fallback = fillSpan(dst, y, l, r, word, options, carrier)
		}
		diagnostic = diagnostic || fallback
	}
	return diagnostic, nil
}

// fillDisc follows resident2275's midpoint span generator. Its horizontal
// radius includes radius>>3; an ideal circular fill loses the source shape.
func fillDisc(dst []byte, center Point, radius int, word uint16, b Rect, options byte, carrier *Carrier) bool {
	return fillDiscOwned(dst, center, radius, word, b, options, carrier, nil, nil)
}
func fillDiscOwned(dst []byte, center Point, radius int, word uint16, b Rect, options byte, carrier *Carrier, owner *spanOwnership, feedback *world.TerrainFeedback) bool {
	layout := rasterlayout.FromPixels(len(dst))
	// Physical high-resolution pixels are square. The original dot was taller
	// on the corrected display; keep both the source disc recipe and its aspect.
	ry := radius
	if layout != nil && layout.Mapped() {
		ry = radius * layout.PictureHeight * Width / (layout.Width * Height)
	}
	pad := radius >> 3
	if center.X-radius-pad > b.Right || center.X+radius < b.Left || center.Y-ry >= b.Bottom || center.Y+ry < b.Top {
		return false
	}
	var s spans
	s.reset(b.Top, b.Bottom)
	set := func(y, half int) {
		if y < b.Top || y > b.Bottom {
			return
		}
		if ry != 0 && ry != radius {
			half = half * radius / ry
		}
		l, r := center.X-half-pad, center.X+half+pad
		if l < b.Left {
			l = b.Left
		}
		if r > b.Right {
			r = b.Right
		}
		s.left[y], s.right[y] = l, r
	}
	x, y, decision := 0, ry, 3-2*ry
	for {
		set(center.Y-x, y)
		set(center.Y+x, y)
		if decision < 0 {
			decision += 4*x + 6
			x++
		} else {
			set(center.Y-y, x)
			set(center.Y+y, x)
			decision += 4*(x-y) + 10
			y--
			x++
		}
		if x >= y {
			break
		}
	}
	if x == y {
		set(center.Y-x, y)
		set(center.Y+x, y)
	}
	diagnostic := false
	for y := b.Top; y <= b.Bottom; y++ {
		if s.left[y] <= s.right[y] && s.left[y] < b.Right && s.right[y] > b.Left {
			owner.span(dst, y, s.left[y], s.right[y], word)
			if feedback != nil {
				fillTerrainFeedbackSpan(dst, y, s.left[y], s.right[y], byte(word), feedback)
			} else {
				diagnostic = fillSpan(dst, y, s.left[y], s.right[y], word, options, carrier) || diagnostic
			}
		}
	}
	return diagnostic
}

// checkerParity is the explicit two-ink VGA span phase in logical coordinates.
func checkerParity(x, y int) int { return (x + y) & 1 }
func fillSpan(dst []byte, y, l, r int, word uint16, options byte, carrier *Carrier) bool {
	if len(dst) > Width*Height*4 {
		return fillMappedSpan(dst, y, l, r, word, options, carrier)
	}
	if len(dst) == Width*Height*4 {
		return fillSpanScaled(dst, y, l, r, word, options, carrier)
	}
	return logicalSpan(dst[y*Width:(y+1)*Width], y, l, r, word, options, carrier)
}
func logicalSpan(dst []byte, y, l, r int, word uint16, options byte, carrier *Carrier) bool {
	lo, hi := byte(word), byte(word>>8)
	if lo == hi && lo != 0 && lo != 0xff {
		row := dst[l : r+1]
		for i := range row {
			row[i] = lo
		}
		return false
	}
	if word == 0 {
		for x := l; x <= r; x++ {
			i := x
			if dst[i] >= 0x60 {
				dst[i] = (dst[i] | 2) - 1
			}
		}
		return false
	}
	if word == 0xffff {
		for x := l; x <= r; x++ {
			i := x
			if dst[i]&0xf0 == 0x60 {
				dst[i] = (dst[i] & 15) | 0x18
			}
		}
		return false
	}
	if lo == 0 {
		door := sourceScreenDoorSpan(y, l, r, 1)
		for x := l; x <= r; x++ {
			if door.writes(x) {
				dst[x] = hi
			}
		}
		return false
	}

	if hi == 0 {
		hi = lo ^ 7
		if options&1 != 0 {
			if !carrier.supportsSpan(y, l, r) {
				// This deterministic hatch is not the recovered memory texture.
				// Keep its indices inside the independently proven source family,
				// including compact scenery (not only OBJFILE Wreck0098).
				for x := l; x <= r; x++ {
					dst[x] = lo&0xfc | byte(((x>>2)^(y>>2))&3)
				}
				return true
			}
			x := l
			a, h := lo, hi
			if (r-l+1)&1 != 0 {
				dst[x] = a
				x++
			}
			for x <= r {
				at := x
				q := (h ^ carrier.CS[y*Width+at] ^ carrier.DS[y*Width+at]) & 3
				a ^= q
				h = q ^ a
				dst[at] = a
				dst[at+1] = h
				x += 2
			}
			return false
		}
	}
	for x := l; x <= r; x++ {
		if checkerParity(x, y) == 0 {
			if lo != 0 {
				dst[x] = lo
			}
		} else {
			dst[x] = hi
		}
	}
	return false
}

// MatchPolygon is CPIT1965's exact destination-index operation, deliberately
// separate from the same-width world material word handled above.
func MatchPolygon(dst []byte, points []Point, word uint16, bounds Rect) error {
	if !targetBoundsValid(dst, bounds) {
		return fmt.Errorf("invalid match polygon target or bounds")
	}
	var clip edgeClipper
	if targetWidth(dst) != Width {
		clip = func(_ int, a, z Point, b Rect) (Point, Point, bool, int, error) { return clipLineTarget(a, z, b, true) }
	}
	var s spans
	if err := s.polygon(points, bounds, clip); err != nil {
		return err
	}
	for y := bounds.Top; y <= bounds.Bottom; y++ {
		l, r := s.left[y], s.right[y]
		if l < bounds.Left {
			l = bounds.Left
		}
		if r > bounds.Right {
			r = bounds.Right
		}
		for x := l; x <= r; x++ {
			i := y*targetWidth(dst) + x
			if dst[i] == byte(word>>8) {
				dst[i] = byte(word)
			}
		}
	}
	return nil
}

func high(v Vector) Vector {
	return Vector{int32(int16(v[0] >> 16)), int32(int16(v[1] >> 16)), int32(int16(v[2] >> 16))}
}
func depthIntersection(a, b Vector, z int32) (Vector, error) {
	dz := intoWord(int(b[2] - a[2]))
	step := intoWord(int(z - a[2]))
	var p Vector
	for i := 0; i < 2; i++ {
		delta := intoWord(int(b[i] - a[i]))
		q, e := idiv(int64(delta)*int64(step), int64(dz))
		if e != nil {
			return p, e
		}
		v := intoWord(int(a[i]) + q)
		p[i] = int32(v) << 16
	}
	p[2] = z << 16
	return p, nil
}

func clipDepthPolygon(points []Vector, near, far int32) ([]Vector, error) {
	return clipDepthPolygonInto(nil, points, near, far)
}
func clipDepthPolygonInto(out, points []Vector, near, far int32) ([]Vector, error) {
	for i, a := range points {
		b := points[(i+1)%len(points)]
		ha, hb := high(a), high(b)
		appendIntersection := func(x, y Vector, z int32) error {
			p, e := depthIntersection(x, y, z)
			if e == nil {
				out = append(out, p)
			}
			return e
		}
		if ha[2] < near {
			if hb[2] >= near {
				if e := appendIntersection(ha, hb, near); e != nil {
					return nil, e
				}
			}
		} else if ha[2] > far {
			if hb[2] <= far {
				if e := appendIntersection(ha, hb, far); e != nil {
					return nil, e
				}
			}
		} else {
			out = append(out, a)
			if hb[2] < near {
				if e := appendIntersection(hb, ha, near); e != nil {
					return nil, e
				}
			} else if hb[2] > far {
				if e := appendIntersection(hb, ha, far); e != nil {
					return nil, e
				}
			}
		}
	}
	return out, nil
}

// Scale2 preserves source material coordinates at320x200 while destination reads
// use the actual640x400 buffer. No high-resolution CS/DS carrier is invented.
func targetWidth(dst []byte) int {
	if l := rasterlayout.FromPixels(len(dst)); l != nil {
		return l.Width
	}
	return Width
}
func targetBoundsValid(dst []byte, b Rect) bool {
	w := targetWidth(dst)
	h := len(dst) / w
	return rasterlayout.FromPixels(len(dst)) != nil && b.Left >= 0 && b.Top >= 0 && b.Right < w && b.Bottom < h && b.Left <= b.Right && b.Top <= b.Bottom
}
func fillSpanScaled(dst []byte, y, l, r int, word uint16, options byte, carrier *Carrier) bool {
	lo, hi := byte(word), byte(word>>8)
	stride := Width * 2
	if lo == hi && lo != 0 && lo != 0xff {
		row := dst[y*stride+l : y*stride+r+1]
		for i := range row {
			row[i] = lo
		}
		return false
	}
	if word == 0 || word == 0xffff {
		for x := l; x <= r; x++ {
			at := y*stride + x
			if word == 0 {
				if dst[at] >= 0x60 {
					dst[at] = (dst[at] | 2) - 1
				}
			} else if dst[at]&0xf0 == 0x60 {
				dst[at] = (dst[at] & 15) | 0x18
			}
		}
		return false
	}
	if lo == 0 {
		door := sourceScreenDoorSpan(y, l, r, 2)
		for x := l; x <= r; x++ {
			if door.writes(x) {
				dst[y*stride+x] = hi
			}
		}
		return false
	}

	if hi == 0 && options&1 != 0 {
		if !carrier.supportsSpan(y/2, l/2, r/2) {
			for x := l; x <= r; x++ {
				dst[y*stride+x] = lo&0xfc | byte((((x/2)>>2)^((y/2)>>2))&3)
			}
			return true
		}
		// Carrier addresses and pair feedback remain source320-stride. Expand the
		// computed logical sample into only its covered physical subpixels.
		first, last := l/2, r/2
		a, h := lo, lo^7
		u := first
		put := func(u int, v byte) {
			for x := max(l, u*2); x <= min(r, u*2+1); x++ {
				dst[y*stride+x] = v
			}
		}
		if (last-first+1)&1 != 0 {
			put(u, a)
			u++
		}
		for u <= last {
			at := (y/2)*Width + u
			q := (h ^ carrier.CS[at] ^ carrier.DS[at]) & 3
			a ^= q
			h = q ^ a
			put(u, a)
			put(u+1, h)
			u += 2
		}
		return false
	}
	if hi == 0 {
		hi = lo ^ 7
	}
	for x := l; x <= r; x++ {
		v := lo
		if checkerParity(x/2, y/2) != 0 {
			v = hi
		}
		dst[y*stride+x] = v
	}

	return hi == 0 && options&1 != 0 && (carrier == nil || len(carrier.CS) < 64000 || len(carrier.DS) < 64000)
}

func expandLogicalSpan(row []byte, l, r int, layout *rasterlayout.Layout, logical []byte) {
	for x := l; x <= r; x++ {
		row[x] = logical[layout.LogicalX[x]]
	}
}
func fillMappedSpan(dst []byte, y, l, r int, word uint16, options byte, carrier *Carrier) bool {
	layout := rasterlayout.FromPixels(len(dst))
	row := dst[y*layout.Width : (y+1)*layout.Width]
	lo, hi := byte(word), byte(word>>8)
	if lo == hi {
		return logicalSpan(row, y, l, r, word, options, carrier)
	}
	first, last := layout.LogicalX[l], layout.LogicalX[r]
	ly := layout.LogicalY[y]
	if lo == 0 {
		door := sourceScreenDoorSpan(ly, first, last, 1)
		for x := l; x <= r; x++ {
			if door.writes(layout.LogicalX[x]) {
				row[x] = hi
			}
		}
		return false
	}
	var logical [Width]byte
	fallback := logicalSpan(logical[:], ly, first, last, word, options, carrier)
	expandLogicalSpan(row, l, r, layout, logical[:])
	return fallback
}
