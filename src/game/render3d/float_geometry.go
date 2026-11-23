package render3d

import (
	"math"
	"slices"

	"gunship2000-assets/game/world"
)

// Camera-space units match the integer renderer's high words. World vertices
// are four times larger; horizontal and vertical view columns have half gain.
type floatVertex [3]float64
type floatMatrix [3][3]float64

type floatWorkspace struct {
	placements         []floatPlacement
	cell, model        []floatVertex // Cell vertices remain live while its BSP visits attached models.
	polygon, temporary []floatVertex
	pixels             []Point
}

func floatRotation(angles [3]uint16, fraction [3]float64, view bool) floatMatrix {
	var sin, cos [3]float64
	for i, a := range angles {
		sin[i], cos[i] = math.Sincos((float64(a) + fraction[i]) * (2 * math.Pi / 65536))
	}
	sa, sb, sc := sin[0], sin[1], sin[2]
	ca, cb, cc := cos[0], cos[1], cos[2]
	m := floatMatrix{
		{cb*cc - sa*sb*sc, cb*sc + sa*sb*cc, -ca * sb},
		{-ca * sc, ca * cc, sa},
		{sb*cc + sa*cb*sc, sb*sc - sa*cb*cc, ca * cb},
	}
	if view {
		for i := range m {
			m[i][0] *= .5
			m[i][1] *= .5
		}
	}
	return m
}

func floatComposeObject(view, object floatMatrix) (out floatMatrix) {
	for i := range out {
		for j := range out[i] {
			for k := 0; k < 3; k++ {
				out[i][j] += object[k][i] * view[k][j]
			}
		}
	}
	return
}

func transformFloat(x, y, z float64, m floatMatrix) floatVertex {
	return floatVertex{(x*m[0][0] + y*m[1][0] + z*m[2][0]) * .25, (x*m[0][1] + y*m[1][1] + z*m[2][1]) * .25, (x*m[0][2] + y*m[1][2] + z*m[2][2]) * .25}
}

func (f *frame) floatTranslation(rel world.Vertex, id string) floatVertex {
	q := f.scene.InstanceFractions[id].Position
	c := f.scene.CameraFraction.Position
	return transformFloat(float64(rel[0])+q[0]-c[0], float64(rel[1])+q[1]-c[1], float64(rel[2])+q[2]-c[2], f.floatView)
}

func (f *frame) floatVertices(out []floatVertex, g *world.Geometry, id string, m floatMatrix, t floatVertex) []floatVertex {
	out = out[:0]
	if g.Generator != nil || g.EmbeddedPreparation == world.EmbeddedPlayerFlare {
		shifts := [3]int{5, 3, 3}
		if g.Generator != nil {
			for i := range shifts {
				shifts[i] = int(g.Generator.Shifts[i])
			}
		}
		x, y, z := math.Ldexp(1, shifts[1]), math.Ldexp(1, shifts[0]), math.Ldexp(1, shifts[2])
		for _, v := range [...]floatVertex{{0, y, 0}, {x, 0, z}, {x, 0, -z}, {-x, 0, z}, {-x, 0, -z}} {
			out = append(out, transformFloat(v[0], v[1], v[2], m))
		}
		// The fourth source corner is a one's complement of the fixed sum.
		for j := range out[4] {
			out[4][j] -= 1.0 / 65536
		}
		if g.Generator != nil {
			for _, delta := range g.Generator.ClusterDeltas {
				last := len(out) - 5
				for i := 0; i < 5; i++ {
					v := out[last+i]
					v[0] += float64(delta[0])
					v[2] += float64(delta[1])
					out = append(out, v)
				}
			}
		}
	} else {
		base := f.vertexStream(g, id)
		out = slices.Grow(out, len(base))
		for _, v := range base {
			x, y, z := float64(v[0]), float64(v[1]), float64(v[2])
			q := floatVertex{x, y, z}
			if g.CompactCallback != 5 && g.EmbeddedPreparation != world.EmbeddedPlayerChaff {
				q = transformFloat(x, y, z, m)
			}
			out = append(out, q)
		}
	}
	for i := range out {
		for j := range t {
			out[i][j] += t[j]
		}
	}
	return out
}

func intersectionFloat(a, b floatVertex, axis int, limit float64) floatVertex {
	t := (limit - a[axis]) / (b[axis] - a[axis])
	for j := range a {
		a[j] += (b[j] - a[j]) * t
	}
	a[axis] = limit
	return a
}

type clipPlane struct {
	axis    int
	limit   float64
	minimum bool
}

func (p clipPlane) outside(v floatVertex) bool {
	if p.minimum {
		return v[p.axis] < p.limit
	}
	return v[p.axis] > p.limit
}

// Clip closed faces without quantizing newly generated vertices. Only a
// crossing consumes the second buffer; interior faces retain their input.
func (w *floatWorkspace) clip(p clipPlane) {
	in := w.polygon
	outs := 0
	for _, v := range in {
		if p.outside(v) {
			outs++
		}
	}
	if outs == 0 {
		return
	}
	if outs == len(in) {
		w.polygon = in[:0]
		return
	}
	out := w.temporary[:0]
	a := in[len(in)-1]
	for _, b := range in {
		if p.outside(a) != p.outside(b) {
			out = append(out, intersectionFloat(a, b, p.axis, p.limit))
		}
		if !p.outside(b) {
			out = append(out, b)
		}
		a = b
	}
	w.polygon, w.temporary = out, in[:0]
}

func clipSegment(a, b *floatVertex, planes []clipPlane) bool {
	for _, p := range planes {
		oa, ob := p.outside(*a), p.outside(*b)
		if oa && ob {
			return false
		}
		if oa {
			*a = intersectionFloat(*a, *b, p.axis, p.limit)
		} else if ob {
			*b = intersectionFloat(*a, *b, p.axis, p.limit)
		}
	}
	return true
}

func (f *frame) projectFloat(v floatVertex, shift uint8) floatVertex {
	layout := f.scene.Layout()
	z := math.Max(8, math.Ldexp(v[2], -int(shift)))
	return floatVertex{float64(f.view.Center.X) + v[0]/z*(256*float64(layout.Width)/Width), float64(f.view.Center.Y) - v[1]/z*(256*float64(layout.PictureHeight)/Height), v[2]}
}

func floatPixel(v floatVertex) Point { return Point{int(math.Round(v[0])), int(math.Round(v[1]))} }

func (f *frame) floatPrimitive(p world.Primitive, outline bool) {
	w := f.fp
	w.polygon = w.polygon[:0]
	for _, i := range p.Indices {
		if i < 0 || i >= len(f.floatPoints) {
			f.stats.status("Prepared vertex reference outside geometry")
			return
		}
		w.polygon = append(w.polygon, f.floatPoints[i])
	}
	f.stats.Primitives++
	if len(w.polygon) == 0 {
		return
	}
	if p.Opcode == 1 {
		v := w.polygon[0]
		if len(w.polygon) != 1 || v[2] <= 16 {
			return
		}
		q := f.projectFloat(v, 0)
		layout := f.scene.Layout()
		pos := floatPixel(q)
		if pos.X < 0 || pos.X >= layout.X[300] || pos.Y < layout.Top || pos.Y >= layout.Y[150] {
			return
		}
		radius := int(256 * float64(p.Radius) * float64(layout.Width) / (Width * v[2]))
		if radius >= 0 && radius <= layout.X[319] {
			f.rasterDisc(p, pos, radius, outline)
		}
		return
	}
	depth := [2]clipPlane{{2, 8, true}, {2, 8192, false}}
	b := f.view.Bounds
	screen := [4]clipPlane{{0, float64(b.Left), true}, {0, float64(b.Right), false}, {1, float64(b.Top), true}, {1, float64(b.Bottom), false}}
	if p.Opcode&0x80 != 0 || outline {
		n := len(w.polygon)
		count := n
		if p.Opcode == 0x82 {
			if n != 2 {
				return
			}
			count = 1
		}
		p.Opcode = 0x82
		for i := 0; i < count; i++ {
			a, b := w.polygon[i], w.polygon[(i+1)%n]
			if !clipSegment(&a, &b, depth[:]) {
				continue
			}
			a, b = f.projectFloat(a, f.view.DepthScale), f.projectFloat(b, f.view.DepthScale)
			if clipSegment(&a, &b, screen[:]) {
				line := [2]Point{floatPixel(a), floatPixel(b)}
				f.rasterPrimitive(p, line[:], outline, true)
			}
		}
		return
	}
	for _, plane := range depth {
		w.clip(plane)
	}
	for i, v := range w.polygon {
		w.polygon[i] = f.projectFloat(v, f.view.DepthScale)
	}
	for _, plane := range screen {
		w.clip(plane)
	}
	if len(w.polygon) < 3 {
		return
	}
	w.pixels = w.pixels[:0]
	for _, v := range w.polygon {
		w.pixels = append(w.pixels, floatPixel(v))
	}
	f.rasterPrimitive(p, w.pixels, outline, true)
}

// Submissions live only until this draw finishes. The shared BSP queue keeps
// an index so growing the workspace never invalidates an enqueued placement.
type floatPlacement struct {
	matrix                 floatMatrix
	translation, localView floatVertex
}

func (f *frame) floatPlacement(relative floatVertex, angles [3]uint16, oriented bool, id string) floatPlacement {
	q := f.scene.InstanceFractions[id]
	camera := f.scene.CameraFraction.Position
	for i := range relative {
		relative[i] += q.Position[i] - camera[i]
	}
	t := transformFloat(relative[0], relative[1], relative[2], f.floatView)
	local := floatVertex{-relative[0], -relative[1], -relative[2]}
	m := f.floatView
	if oriented {
		rotation := floatRotation(angles, q.Angles, false)
		local = transformFloat(local[0], local[1], local[2], rotation)
		for i := range local {
			local[i] *= 4
		}
		m = floatComposeObject(m, rotation)
	}
	return floatPlacement{m, t, local}
}

func (f *frame) floatCulled(g *world.Geometry, v floatVertex, cell, center bool) bool {
	_, _, limit := f.thresholds()
	depth := v[2] + float64(g.Extent)
	if depth < 0 || depth >= float64(limit) {
		return true
	}
	yLimit := depth * .5
	if cell {
		if !center {
			depth *= .5
		}
		yLimit = depth
	}
	return math.Abs(v[0]) > depth || math.Abs(v[1]) > yLimit
}

func floatPlaneNegative(p world.Plane, v floatVertex) bool {
	// Keep the source plane-side bias in the original coefficient units.
	return float64(p[0])*v[0]+float64(p[1])*v[1]+float64(p[2])*v[2]+float64(p[3]) < -0x20000
}

func (f *frame) placedItem(g *world.Geometry, origin, offset world.Vertex, angles [3]uint16, oriented bool, callback uint8, id string) (drawItem, bool) {
	if f.fp == nil {
		return f.item(g, world.Vertex{origin[0] + offset[0], origin[1] + offset[1], origin[2] + offset[2]}, angles, oriented, callback, id)
	}
	var relative floatVertex
	for i := range relative {
		relative[i] = float64(origin[i]) + float64(offset[i])
	}
	near, outline, _ := f.thresholds()
	if g.EmbeddedPreparation != world.EmbeddedOrdinary && math.Ldexp((math.Abs(relative[0])+math.Abs(relative[2]))*.5, -int(f.view.DepthScale)) >= float64(near) {
		f.stats.Rejected++
		return drawItem{}, false
	}
	placement := f.floatPlacement(relative, angles, oriented, id)
	v := placement.translation
	if f.floatCulled(g, v, false, false) {
		f.stats.Rejected++
		return drawItem{}, false
	}
	metric := math.Abs(v[0]) + float64(g.Extent) + v[2] - v[1]
	index := len(f.fp.placements)
	f.fp.placements = append(f.fp.placements, placement)
	return drawItem{geometry: g, callback: callback, id: id, metric: int16(math.Min(32767, math.Max(-32768, metric))), outline: metric >= float64(outline), floatIndex: index}, true
}
