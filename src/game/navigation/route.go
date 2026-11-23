// Package navigation owns transient source-compatible routes and the normal GPS
// modal. It neither generates missions nor persists pilot preferences.
package navigation

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
)

const PointCount = 3

type Point struct {
	H0, H1 uint32
	Tail   uint16
}

func (p Point) Present() bool { return p.H0 != 0 }

// Route keeps all70 source bytes: points occupy+10/+20/+30, not the last30
// formation/order/cache bytes. The opaque tails survive clearing and edits.
type Route struct{ Raw [70]byte }

func (r *Route) Index() int8     { return int8(r.Raw[0]) }
func (r *Route) SetIndex(n int8) { r.Raw[0] = byte(n) }
func (r *Route) Point(n int) Point {
	if n < 1 || n > 3 {
		return Point{}
	}
	at := n * 10
	return Point{binary.LittleEndian.Uint32(r.Raw[at:]), binary.LittleEndian.Uint32(r.Raw[at+4:]), binary.LittleEndian.Uint16(r.Raw[at+8:])}
}
func (r *Route) SetPoint(n int, p Point) error {
	if n < 1 || n > 3 {
		return fmt.Errorf("route point%d outside1..3", n)
	}
	at := n * 10
	binary.LittleEndian.PutUint32(r.Raw[at:], p.H0)
	binary.LittleEndian.PutUint32(r.Raw[at+4:], p.H1)
	binary.LittleEndian.PutUint16(r.Raw[at+8:], p.Tail)
	return nil
}
func (r *Route) ClearPoints() {
	for n := 1; n <= 3; n++ {
		p := r.Point(n)
		p.H0, p.H1 = 0, 0
		_ = r.SetPoint(n, p)
	}
}
func (r *Route) Next() {
	if r.Index() == 0 {
		return
	}
	n := r.Index() + 1
	if n > 3 || !r.Point(int(n)).Present() {
		n = 1
	}
	r.SetIndex(n)
}
func (r *Route) Previous() {
	// Original0e16 tests the raw row's first dword when selector0. It is not
	// symmetric with Next, and does not test a fictional fourth waypoint.
	if r.Index() == 0 && binary.LittleEndian.Uint32(r.Raw[:]) != 0 {
		return
	}
	n := r.Index() - 1
	if n < 1 {
		n = 3
	}
	r.SetIndex(n)
}
func (r *Route) ToFlight(s *flight.State) {
	s.WaypointIndex = r.Index()
	for n := 1; n <= 3; n++ {
		p := r.Point(n)
		s.Waypoints[n] = flight.Waypoint{H0: p.H0, H1: p.H1}
	}
}
func (r *Route) FromFlight(s *flight.State)          { r.SetIndex(s.WaypointIndex) }
func (r *Route) Home(home, farp Point, present bool) { r.HomeFrom(home, home, farp, present) }
func (r *Route) HomeFrom(base, home, farp Point, present bool) {
	chosen := home
	if present && r.Point(1).H0 == base.H0 {
		chosen = farp
	}
	chosen.Tail = r.Point(1).Tail
	_ = r.SetPoint(1, chosen)
	for n := 2; n <= 3; n++ {
		p := r.Point(n)
		p.H0, p.H1 = 0, 0
		_ = r.SetPoint(n, p)
	}
	r.SetIndex(1)
}

// Bearing uses the original signed-word SAR4 coordinate domain. Display style23
// divides the negative source angle by183; style19 distance is the source metric.
func Bearing(from Point, to Point, a *world.Bundle) (uint16, uint16) {
	dx := int16(int32(to.H0)>>4) - int16(int32(from.H0)>>4)
	dz := int16(int32(to.H1)>>4) - int16(int32(from.H1)>>4)
	angle := uint16(0x4000 - world.SourceAngle(dz, dx, a))
	abs := func(n int16) int16 {
		if n < 0 {
			return -n
		}
		return n
	}
	x, z := abs(dx), abs(dz)
	if x < z {
		x, z = z, x
	}
	return angle, uint16(2*x + z)
}

// ResolveHome is GPS4:10c4..118b. An absolute own-home is distinct from the
// TARGET base used by the FARP toggle. Relative rows borrow leaderH0 and add
// ownH1 to leaderH1; there is no guessed current-aircraft position fallback.
func ResolveHome(rows [][32]byte, control [70]byte, slot int) (Point, error) {
	if slot < 0 || slot >= len(rows) {
		return Point{}, fmt.Errorf("GPS home row%d unavailable", slot)
	}
	point := func(row [32]byte) Point {
		return Point{H0: binary.LittleEndian.Uint32(row[2:]), H1: binary.LittleEndian.Uint32(row[6:])}
	}
	own := point(rows[slot])
	if int32(own.H0) > 0x400 {
		return own, nil
	}
	leader := int(int8(control[5]))
	if leader < 0 || leader >= len(rows) {
		return Point{}, fmt.Errorf("GPS relative home has unsupported leader%d", leader)
	}
	base := point(rows[leader])
	return Point{H0: base.H0, H1: base.H1 + own.H1}, nil
}
