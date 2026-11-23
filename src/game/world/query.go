package world

import "fmt"

// QueryPose uses native horizontal units and the source signed height word.
// Visual replacement meshes never participate in these retained query regions.
type QueryPose struct {
	H0, H1 uint32
	Height int16
}

type QueryObject struct {
	ModelID int
	Pose    QueryPose
}

// SourceContains reproduces resident037f:0cd0's crossing parity. Boundary
// equality is accepted on the divided mixed-sign edge, not by a generic epsilon.
func SourceContains(points [][2]int16, x, z int16) (bool, error) {
	if len(points) < 4 || points[0] != points[len(points)-1] {
		return false, fmt.Errorf("query polygon must be explicitly closed")
	}
	count := 0
	oldX, oldZ := points[0][0]-x, points[0][1]-z
	for _, p := range points[1:] {
		newX, newZ := p[0]-x, p[1]-z
		if uint16(newZ^oldZ)&0x8000 != 0 && uint16(oldX&newX)&0x8000 == 0 {
			if uint16(oldX|newX)&0x8000 == 0 {
				count++
			} else {
				n, d := int32(newX-oldX)*int32(oldZ), int32(newZ-oldZ)
				if d == 0 {
					return false, fmt.Errorf("source query edge division by zero")
				}
				q := n / d
				if q < -32768 || q > 32767 {
					return false, fmt.Errorf("source query edge division overflow")
				}
				if int16(q) == oldX {
					return true, nil
				}
				if int16(q) < oldX {
					count++
				}
			}
		}
		oldX, oldZ = newX, newZ
	}
	return count&1 != 0, nil
}

func RegionHeight(regions []HeightRegion, x, z, current int16) (int16, error) {
	for _, r := range regions {
		if r.Threshold != 0 && r.Threshold > current {
			continue
		}
		inside, err := SourceContains(r.Points, x, z)
		if err != nil {
			return 0, err
		}
		if !inside {
			continue
		}
		// IMUL/ADD wrap signed32 before source IDIV; NEG wraps the final word.
		n := int32(int64(r.Plane[0])*int64(x) + int64(r.Plane[2])*int64(z) + int64(r.Plane[3]))
		d := r.Plane[1]
		if d == 0 {
			return 0, fmt.Errorf("height query plane divisor is zero")
		}
		q := int64(n) / int64(d)
		if q < -32768 || q > 32767 {
			return 0, fmt.Errorf("height query plane division overflow")
		}
		return -int16(q), nil // First matching region; no invented maximum rule.
	}
	return 0, nil
}

type OutsideGridError struct{ H0, H1 uint32 }

func (e OutsideGridError) Error() string {
	return fmt.Sprintf("height query coordinates (%d,%d) outside source grid", e.H0, e.H1)
}

// SourceCellID keeps the same grid domain and reversed H1 axis as037f:2b6c.
func (b *Bundle) SourceCellID(worldID string, pose QueryPose) (byte, error) {
	w := b.Worlds[worldID]
	if w == nil || len(w.Grid) != 4096 {
		return 0, fmt.Errorf("height query world%s is incomplete", worldID)
	}
	x, z := int(pose.H0>>13), int(pose.H1>>13)
	if x < 0 || x >= 64 || z < 0 || z >= 64 {
		return 0, OutsideGridError{pose.H0, pose.H1}
	}
	return w.Grid[x+64*(63-z)], nil
}

func (b *Bundle) queryCell(worldID string, pose QueryPose) (*Geometry, int16, int16, error) {
	cellID, err := b.SourceCellID(worldID, pose)
	if err != nil {
		return nil, 0, 0, err
	}
	w := b.Worlds[worldID]
	if len(w.Cells) != 64 {
		return nil, 0, 0, fmt.Errorf("height query world%s has incomplete cells", worldID)
	}
	cell := int(cellID)
	if cell >= len(w.Cells) {
		return nil, 0, 0, fmt.Errorf("height query cell%d outside definitions", cell)
	}
	g := b.Geometries[w.Cells[cell].Geometry]
	if g == nil {
		return nil, 0, 0, fmt.Errorf("height query cell geometry missing")
	}
	return g, int16(pose.H0&0x1fff) - 0x1000, int16(pose.H1&0x1fff) - 0x1000, nil
}

// TerrainHeight is resident037f:31e3. It does not include scenery artwork or
// model regions; its native terrain shape is independent of visual tessellation.
func (b *Bundle) TerrainHeight(worldID string, pose QueryPose) (int16, error) {
	g, x, z, err := b.queryCell(worldID, pose)
	if err != nil {
		return 0, err
	}
	return RegionHeight(g.Heights, x, z, pose.Height)
}

// ContactHeight is resident037f:314c: source-ordered model regions in the same
// cell first, then terrain. Model query regions are deliberately not rotated:
// the original subtracts placement offsets but never reads the angle extension.
func (b *Bundle) ContactHeight(worldID string, pose QueryPose, objects []QueryObject) (int16, error) {
	g, x, z, err := b.queryCell(worldID, pose)
	if err != nil {
		return 0, err
	}
	for _, o := range objects {
		if o.Pose.H0>>13 != pose.H0>>13 || o.Pose.H1>>13 != pose.H1>>13 {
			continue
		}
		if o.ModelID < 0 || o.ModelID >= len(b.Models) {
			return 0, fmt.Errorf("contact model ID%d outside prepared directory", o.ModelID)
		}
		model := b.Geometries[b.Models[o.ModelID]]
		if model == nil {
			return 0, fmt.Errorf("contact model%d geometry missing", o.ModelID)
		}
		localX := int16((pose.H0&0x1fff)|1) - int16(o.Pose.H0&0x1fff)
		localZ := int16((pose.H1&0x1fff)|1) - int16(o.Pose.H1&0x1fff)
		y, e := RegionHeight(model.Heights, localX, localZ, pose.Height)
		if e != nil {
			return 0, e
		}
		if y != 0 {
			return y, nil
		}
	}
	return RegionHeight(g.Heights, x|1, z|1, pose.Height)
}
