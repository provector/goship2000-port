package cockpit

// ProjectilePoint is immutable logical320 display space after the source range
// shift and heading rotation. Cross is already gated by the logical blink phase.
type ProjectilePoint struct {
	X, Y  int16
	Cross bool
}

func (a *Assets) threatGeometry(aircraft int) (cx, cy int, clip Rect, ok bool) {
	f, err := Family(aircraft)
	if err != nil {
		return
	}
	d, found := a.Definitions[f]
	if !found {
		return
	}
	p := func(off int) int { return int(d.Parameters[off/2]) }
	return p(0x5c), p(0x5e), Rect{p(0x60), p(0x62), p(0x64), p(0x66)}, true
}

// DrawThreatContactPoint is shared only by ordered logical feedback and pure
// composition. Each accepted contact changes the destination seen by later dots.
func (a *Assets) DrawThreatContactPoint(dst []byte, aircraft int, phase uint16, q ContactPoint) bool {
	cx, cy, clip, ok := a.threatGeometry(aircraft)
	if !ok || !LogicalTarget(dst) {
		return false
	}
	x, y := cx+int(q.X), cy+int(q.Y)
	if x < clip.Left || x > clip.Right || y < clip.Top || y > clip.Bottom || x < 0 || x >= 320 || y < 0 || y >= 200 || dst[y*320+x] >= 0x50 {
		return false
	}
	dot := func(x, y int, c byte) {
		if x >= 0 && x < 320 && y >= 0 && y < 200 {
			pixel(dst, x, y, c, clip)
		}
	}
	dot(x, y, q.Colour)
	if q.Attack && phase&2 != 0 {
		dot(x-1, y, 13)
		dot(x+1, y, 13)
		dot(x, y-1, 13)
		dot(x, y+1, 13)
	}
	return true
}

// DrawThreatProjectilePoint mirrors source3b42..3bea. Unlike contacts, there is
// no CPIT rectangle test here: the range test happened before rotation. Only the
// center reads the destination index; accepted cross neighbors are raw writes.
func (a *Assets) DrawThreatProjectilePoint(dst []byte, aircraft int, q ProjectilePoint) bool {
	cx, cy, _, ok := a.threatGeometry(aircraft)
	if !ok || !LogicalTarget(dst) {
		return false
	}
	x, y := cx+int(q.X), cy+int(q.Y)
	address := func(x, y int) int { return int(uint16(uint16(byte(y))*320 + uint16(x))) }
	i := address(x, y)
	// DOS could address its full64KiB page. Native support is deliberately
	// bounded to the established visible64000 bytes, never arbitrary memory.
	if i >= 64000 || dst[i] >= 0x50 {
		return false
	}
	PutLogical(dst, i, 14)
	if q.Cross {
		for _, pt := range [4][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
			if j := address(pt[0], pt[1]); j < 64000 {
				PutLogical(dst, j, 14)
			}
		}
	}
	return true
}
