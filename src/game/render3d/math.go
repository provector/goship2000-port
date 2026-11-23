package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
)

func Sine(angle uint16, quarter []int16) int16 {
	phase := int(angle >> 4)
	i := phase & 1023
	if phase&1024 != 0 {
		i = 1024 - i
	}
	v := quarter[i]
	if phase&2048 != 0 {
		return -v
	}
	return v
}

func qmul(a, b int16) int16 { return int16((int32(a) * int32(b)) >> 14) }

func Rotation(angles [3]uint16, quarter []int16, view bool) Matrix {
	sa, sb, sc := Sine(angles[0], quarter), Sine(angles[1], quarter), Sine(angles[2], quarter)
	ca, cb, cc := Sine(angles[0]+0x4000, quarter), Sine(angles[1]+0x4000, quarter), Sine(angles[2]+0x4000, quarter)
	t, u := qmul(sa, sb), qmul(sa, cb)
	m := Matrix{
		{qmul(cb, cc) - qmul(t, sc), qmul(cb, sc) + qmul(t, cc), -qmul(ca, sb)},
		{-qmul(ca, sc), qmul(ca, cc), sa},
		{qmul(sb, cc) + qmul(u, sc), qmul(sb, sc) - qmul(u, cc), qmul(ca, cb)},
	}
	if view {
		for row := range m {
			m[row][0] >>= 1
			m[row][1] >>= 1
		}
	}
	return m
}

func Transform(v world.Vertex, m Matrix) Vector {
	var out Vector
	for col := 0; col < 3; col++ {
		for row := 0; row < 3; row++ {
			out[col] += int32(v[row]) * int32(m[row][col])
		}
	}
	return out
}

func transpose(m Matrix) Matrix {
	return Matrix{{m[0][0], m[1][0], m[2][0]}, {m[0][1], m[1][1], m[2][1]}, {m[0][2], m[1][2], m[2][2]}}
}

func compose(a, b Matrix) Matrix {
	var out Matrix
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			var n int32
			for k := 0; k < 3; k++ {
				n += int32(a[i][k]) * int32(b[k][j])
			}
			out[i][j] = int16(n >> 14)
		}
	}
	return out
}

func planeSide(p world.Plane, v world.Vertex) int8 {
	n := int32(p[0])*int32(v[0]) + int32(p[1])*int32(v[1]) + int32(p[2])*int32(v[2]) + p[3]
	return int8(uint32(n+0x20000) >> 24)
}

func idiv(n, d int64) (int, error) {
	if d == 0 {
		return 0, fmt.Errorf("source signed division by zero")
	}
	q := n / d
	if q < -32768 || q > 32767 {
		return 0, fmt.Errorf("source signed division overflow")
	}
	return int(q), nil
}

func wordChecked(n int) (int, error) {
	if n < -32768 || n > 32767 {
		return 0, fmt.Errorf("source signed-word overflow")
	}
	return n, nil
}

// Resident201a is the installed INT4 handler: signed overflow returns the
// appropriate signed-word endpoint. Call only at source INTO sites.
func intoWord(n int) int {
	if n < -32768 {
		return -32768
	}
	if n > 32767 {
		return 32767
	}
	return n
}

// Resident1f8c recognizes projection's exact F7FF (IDIV DI) instruction.
// Its overflow recovery is a second division of DX:AX shifted right8,
// followed by the source +/-64 test and +/-32000 coarse result.
func projectionQuotient(n int32, d int16) (int, error) {
	q, err := idiv(int64(n), int64(d))
	if err == nil || d == 0 {
		return q, err
	}
	coarse, err := idiv(int64(n>>8), int64(d))
	if err != nil {
		return 0, err
	}
	if coarse < -64 {
		return -32000, nil
	}
	if coarse > 64 {
		return 32000, nil
	}
	return int(int16(coarse << 8)), nil
}

func project(v Vector, center Point) (Point, error) {
	return projectWith(v, center, true)
}

func projectWith(v Vector, center Point, saturateCenter bool) (Point, error) {
	z := int16(v[2] >> 16)
	x, err := projectionQuotient(v[0]>>8, z)
	if err != nil {
		return Point{}, err
	}
	y, err := projectionQuotient(v[1]>>8, z)
	if err != nil {
		return Point{}, err
	}
	// NEG before ADD has no INTO; only the subsequent center additions do.
	y = int(int16(-y))
	if saturateCenter {
		return Point{intoWord(center.X + x), intoWord(center.Y + y)}, nil
	}
	return Point{int(int16(center.X + x)), int(int16(center.Y + y))}, nil
}

func CellIndex(h0, h1 uint32) int {
	x, z := uint16((h0<<3)>>16), uint16((h1<<3)>>16)
	return int(uint16(0xfc0 - 64*(z&255) + x))
}

// Register reproduces 2d7f's checked full-width differences and near-boundary
// camera-cell substitution. It returns a raw grid index, not the 0eh-prefixed
// DOS address or the distinct increasing-row height-query key.
func Register(camera Camera, pose Pose) (int, world.Vertex, bool) {
	dx, dz := int32(camera.H0-pose.H0), int32(camera.H1-pose.H1)
	if dx != int32(int16(dx)) || dz != int32(int16(dz)) {
		return 0, world.Vertex{}, false
	}
	abs := func(v int16) uint16 {
		if v < 0 {
			return uint16(-v)
		}
		return uint16(v)
	}
	half := (uint32(abs(int16(dx))) + uint32(abs(int16(dz)))) >> 1
	a0, a1 := pose.H0, pose.H1
	if int16(half) <= 0x400 && (((byte(pose.H0>>8)+1)&14) == 0 || ((byte(pose.H1>>8)+1)&14) == 0) {
		a0, a1 = camera.H0, camera.H1
	}
	local := world.Vertex{int16((a0&8191)-4096) + int16(pose.H0-a0), int16(pose.Height), int16((a1&8191)-4096) + int16(pose.H1-a1)}
	return CellIndex(a0, a1), local, true
}

// Experimental scale2 multiplies the projection numerator before division, not
// the already rounded320x200 vertex. Source overflow recovery stays in scale1.
func (f *frame) project(v Vector, saturate bool) (Point, error) {
	l := f.scene.Layout()
	if l.Width == Width {
		return projectWith(v, f.view.Center, saturate)
	}
	z := int16(v[2] >> 16)
	if z == 0 {
		return Point{}, fmt.Errorf("projection zero depth")
	}
	x := int((int64(v[0]) * int64(l.Width)) / ((int64(z) << 8) * Width))
	y := int((int64(v[1]) * int64(l.PictureHeight)) / ((int64(z) << 8) * Height))
	// The physical640x400 clipper uses wide intermediates. Narrowing here would
	// change an edge's direction before it intersects the actual viewport.
	return Point{f.view.Center.X + x, f.view.Center.Y - y}, nil
}
