package render3d

import (
	"gunship2000-assets/game/world"
	"math"
)

// This is a native, display-only external-camera clearance rule.
// The source orbit may pass behind/through nearby scenery. Retract along that
// orbit before an actual opaque face; never suppress its legitimate geometry.

type observerVector [3]float64

func observerSub(a, b observerVector) observerVector {
	return observerVector{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}
func observerDot(a, b observerVector) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func observerCross(a, b observerVector) observerVector {
	return observerVector{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func observerTriangle(start, end, a, b, c observerVector) (float64, bool) {
	direction := observerSub(end, start)
	e1, e2 := observerSub(b, a), observerSub(c, a)
	p := observerCross(direction, e2)
	det := observerDot(e1, p)
	if math.Abs(det) < 1e-9 {
		return 0, false
	}
	from := observerSub(start, a)
	u := observerDot(from, p) / det
	if u < 0 || u > 1 {
		return 0, false
	}
	q := observerCross(from, e1)
	v := observerDot(direction, q) / det
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := observerDot(e2, q) / det
	return t, t > 0 && t < 1
}

func (r *Renderer) ObserverCamera(scene Scene, focus Instance) Camera {
	original := scene.Camera
	camera := original
	if r == nil || r.Assets == nil || len(r.Assets.Sine) < 1025 || focus.ID == "" {
		return camera
	}
	type obstacle struct {
		in      Instance
		g       *world.Geometry
		inverse Matrix
	}
	var obstacles []obstacle
	for _, in := range scene.Instances {
		if in.ID == focus.ID || in.Impact || in.Explosion {
			continue
		}
		id := in.GeometryID
		if id == "" {
			if in.ModelID < 0 || in.ModelID >= len(r.Assets.Models) {
				continue
			}
			id = r.Assets.Models[in.ModelID]
		}
		g := r.Assets.Geometries[id]
		// Existing support metadata admits scenery, not the viewed aircraft, other
		// helicopters, particles or an invented all-object collision volume.
		if g == nil || len(g.Heights) == 0 || g.Generator != nil || g.CompactCallback != 0 {
			continue
		}
		m := Matrix{{16384, 0, 0}, {0, 16384, 0}, {0, 0, 16384}}
		if in.oriented() {
			m = Rotation(in.Angles, r.Assets.Sine, false)
		}
		obstacles = append(obstacles, obstacle{in, g, m})
	}
	nearest := func(end Pose) (float64, bool) {
		first := 1.0
		hit := false
		for _, o := range obstacles {
			local := func(p Pose) (observerVector, bool) {
				x, z := int64(p.H0)-int64(o.in.H0), int64(p.H1)-int64(o.in.H1)
				y := int64(int16(p.Height)) - int64(int16(o.in.Height))
				if x < -32768 || x > 32767 || y < -32768 || y > 32767 || z < -32768 || z > 32767 {
					return observerVector{}, false
				}
				raw := observerVector{float64(x), float64(y), float64(z)}
				var out observerVector
				for col := 0; col < 3; col++ {
					for row := 0; row < 3; row++ {
						out[col] += raw[row] * float64(o.inverse[row][col]) / 16384
					}
				}
				return out, true
			}
			start, ok := local(focus.Pose)
			if !ok {
				continue
			}
			finish, ok := local(end)
			if !ok {
				continue
			}
			var group func(world.Group)
			group = func(g world.Group) {
				if p := g.Primitive; p != nil && (p.Opcode == 3 || p.Opcode == 4) && p.Colour != 0xffff && byte(p.Colour) != 0 && len(p.Indices) >= 3 {
					valid := true
					for _, i := range p.Indices {
						if i < 0 || i >= len(o.g.Vertices) {
							valid = false
							break
						}
					}
					if valid {
						vertex := func(i int) observerVector {
							v := o.g.Vertices[i]
							return observerVector{float64(v[0]), float64(v[1]), float64(v[2])}
						}
						a := vertex(p.Indices[0])
						for i := 1; i+1 < len(p.Indices); i++ {
							if t, yes := observerTriangle(start, finish, a, vertex(p.Indices[i]), vertex(p.Indices[i+1])); yes && t < first {
								first, hit = t, true
							}
						}
					}
				}
				for _, child := range g.Children {
					group(child)
				}
			}
			if len(o.g.Children) == 0 {
				for _, g := range o.g.DirectGroups {
					group(g)
				}
			} else {
				seen := map[int]bool{}
				var visit func(int)
				visit = func(node int) {
					if node < 0 || node == 255 || node >= len(o.g.Children) || seen[node] {
						return
					}
					seen[node] = true
					if node < len(o.g.PlaneGroups) {
						group(o.g.PlaneGroups[node])
					}
					for side, child := range o.g.Children[node] {
						if edge := o.in.GraphChild; edge != nil && edge.Node == node && edge.Side == side && child == edge.Expected {
							child = edge.Child
						}
						visit(child)
					}
				}
				visit(o.g.Root)
			}
		}
		return first, hit
	}
	// Preserve the source vertical floor. Recheck the shorter sightline because
	// retaining its height can expose another face. Work is bounded and uses no
	// simulation state, random draw, host-time accumulator or saved camera state.
	for pass := 0; pass < 8; pass++ {
		t, hit := nearest(camera.Pose)
		if !hit {
			return camera
		}
		dx, dz := float64(int64(camera.H0)-int64(focus.H0)), float64(int64(camera.H1)-int64(focus.H1))
		dy := float64(int16(camera.Height)) - float64(int16(focus.Height))
		length := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if length == 0 {
			return original
		}
		t -= 32 / length // Source near depth8 corresponds to32 world units.
		if t <= 0 || t >= 1 {
			return original
		} // No proven orbit clearance; never collapse onto focus.
		next := camera
		next.H0 = uint32(int64(focus.H0) + int64(math.Round(dx*t)))
		next.H1 = uint32(int64(focus.H1) + int64(math.Round(dz*t)))
		if next.Pose == camera.Pose {
			return original
		}
		camera = next
	}
	return original // An unresolved multi-surface case preserves the established view.
}
