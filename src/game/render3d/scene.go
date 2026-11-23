package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/rasterlayout"
	"slices"
)

type drawItem struct {
	floatIndex  int
	impact      bool
	geometry    *world.Geometry
	relative    world.Vertex
	localView   world.Vertex
	matrix      Matrix
	translation Vector
	metric      int16
	outline     bool
	callback    uint8
	id          string
	graphChild  *GraphChildOverride
}

type cellOffset struct{ x, z int }

var outerCells = []cellOffset{{-2, 2}, {2, 2}, {2, -2}, {-2, -2}, {-1, 2}, {1, 2}, {0, 2}, {-1, -2}, {1, -2}, {0, -2}, {-2, 1}, {-2, -1}, {-2, 0}, {2, 1}, {2, -1}, {2, 0}}
var innerCells = []cellOffset{{-1, 1}, {1, 1}, {1, -1}, {-1, -1}, {0, 1}, {0, -1}, {-1, 0}, {1, 0}, {0, 0}}

type frame struct {
	fp          *floatWorkspace
	floatView   floatMatrix
	floatPoints []floatVertex
	// Serial primitive submission owns these temporary vertices until both indexed and ownership writers finish.
	verticesScratch, clippedScratch []Vector
	projectedScratch                []Point
	sparseOwners                    bool  // Explicit normal-display mode; model previews and direct diagnostic frames retain full ownership.
	previewClipFlags                *byte // Optional frame-local target-preview feedback; never simulation state.
	r                               *Renderer
	scene                           Scene
	state                           *VisualState
	dst                             []byte
	stats                           Stats
	view                            View
	matrix                          Matrix
	compact                         map[int][]Vector
	visit                           func(drawItem)
	edges                           map[byte]screenEdge
	styleMask                       []byte // Accepted recipe admission only; actual ownership uses canonical spans.
	material                        materialBinding
	compactCallback                 int
	geometryID                      string // Current immutable geometry owner; presentation admission only.
}

type screenEdge struct {
	first  int
	a, b   Point
	ok     bool
	reason int
}

func (f *frame) edgeClipper(flags []byte, refs []int) edgeClipper {
	return func(i int, a, b Point, bounds Rect) (Point, Point, bool, int, error) {
		id := byte(0)
		if i < len(flags) {
			id = flags[i] + 1 // FFh is the source no-reuse sentinel.
		}
		ref := -1
		if i < len(refs) {
			ref = refs[i]
		}
		if old, ok := f.edges[id]; id != 0 && ok {
			if old.first == ref {
				return old.a, old.b, old.ok, old.reason, nil
			}
			return old.b, old.a, old.ok, old.reason, nil
		}

		if f.previewClipFlags != nil && !((a.X < bounds.Left && b.X < bounds.Left) || (a.X > bounds.Right && b.X > bounds.Right) || (a.Y < bounds.Top && b.Y < bounds.Top) || (a.Y > bounds.Bottom && b.Y > bounds.Bottom)) {
			if (a.X < bounds.Left) != (b.X < bounds.Left) {
				*f.previewClipFlags |= 1
			}
			if (a.X > bounds.Right) != (b.X > bounds.Right) {
				*f.previewClipFlags |= 2
			}
		}
		x, y, ok, reason, err := clipLineTarget(a, b, bounds, f.scene.Layout().Width != Width)
		if id != 0 && err == nil {
			f.edges[id] = screenEdge{ref, x, y, ok, reason}
		}
		return x, y, ok, reason, err
	}
}

func (r *Renderer) validateScene(scene Scene) error {
	if scene.Resolution != "" && rasterlayout.ByName(scene.Resolution) == nil {
		return fmt.Errorf("unsupported scene resolution %q", scene.Resolution)
	}
	if scene.RenderScale != 0 && scene.RenderScale != 1 && scene.RenderScale != 2 {
		return fmt.Errorf("render scale must be1 or2")
	}
	w := r.Assets.Worlds[scene.World]
	if w == nil || len(w.Grid) != 4096 || len(w.Cells) != 64 {
		return fmt.Errorf("prepared world %q is unavailable or incompatible", scene.World)
	}
	if !boundsValid(scene.view().Bounds) {
		return fmt.Errorf("invalid scene viewport")
	}
	if CellIndex(scene.Camera.H0, scene.Camera.H1) < 0 || CellIndex(scene.Camera.H0, scene.Camera.H1) >= 4096 {
		return fmt.Errorf("camera lies outside prepared world grid")
	}
	for _, in := range scene.Instances {
		if in.Impact {
			if r.Assets.Combat == nil {
				return fmt.Errorf("impact sprites are not prepared")
			}
			continue
		}
		if in.GeometryID != "" {
			if r.Assets.Geometries[in.GeometryID] == nil {
				return fmt.Errorf("embedded geometry%s unavailable", in.GeometryID)
			}
		} else if in.ModelID < 0 || in.ModelID >= len(r.Assets.Models) {
			return fmt.Errorf("instance %s model ID %d is invalid", in.ID, in.ModelID)
		}
		if in.MutableModel != nil {
			if _, err := in.mutableGeometry(r.Assets.Geometries[r.Assets.Models[in.ModelID]]); err != nil {
				return fmt.Errorf("instance %s mutable model: %w", in.ID, err)
			}
		}
		if in.Callback > 7 {
			return fmt.Errorf("instance %s appearance callback %d is unsupported", in.ID, in.Callback)
		}
		if edge := in.GraphChild; edge != nil {
			g := r.Assets.Geometries[r.Assets.Models[in.ModelID]]
			if g == nil || edge.Node < 0 || edge.Node >= len(g.Children) || edge.Side < 0 || edge.Side > 1 || g.Children[edge.Node][edge.Side] != edge.Expected || (edge.Child != 255 && edge.Child != edge.Expected) {
				return fmt.Errorf("instance %s component edge does not match prepared graph", in.ID)
			}
		}
	}
	return nil
}

func (r *Renderer) Draw(scene Scene, state *VisualState, dst []byte) (Stats, error) {
	if len(dst) != scene.TargetPixels() {
		return Stats{}, fmt.Errorf("scene target size does not match selected render scale")
	}
	if err := r.validateScene(scene); err != nil {
		return Stats{}, err
	}
	for _, mask := range [][]byte{scene.SkyMask, scene.GroundOwners} {
		if mask != nil {
			if len(mask) != len(dst) {
				return Stats{}, fmt.Errorf("material presentation plane dimensions differ")
			}
			clear(mask)
		}
	}
	f := frame{r: r, scene: scene, state: state, dst: dst, view: scene.rasterView(), matrix: Rotation(scene.Camera.Angles, r.Assets.Sine, true), compact: map[int][]Vector{}}
	if scene.FloatingGeometry {
		f.fp = &r.floatScratch
		f.fp.placements = f.fp.placements[:0]
		f.floatView = floatRotation(scene.Camera.Angles, scene.CameraFraction.Angles, true)
	}
	f.sparseOwners = scene.GroundOwners == nil
	// Canonical ownership and accepted recipe admission have different jobs.
	// The former shares actual raster spans. The latter retains the established
	// approximation's conservative footprint, including its clipped edge pixels.
	if scene.SkyMask != nil || scene.GroundOwners != nil {
		if len(r.ownerScratch) != len(dst) {
			r.ownerScratch = make([]byte, len(dst))
		} else {
			clear(r.ownerScratch)
		}
		f.scene.SkyMask = r.ownerScratch
	}
	if scene.GraphicsOptions&2 != 0 {
		if err := f.background(); err != nil {
			clear(f.scene.SkyMask)
			f.stats.status("Background gradient input outside source arithmetic domain; explicit A6 clear used.")
			for i := range dst {
				dst[i] = 0xa6
			}
		}
	} else {
		for i := range dst {
			dst[i] = 0xa6
		}
	}
	if scene.SkyMask != nil {
		if len(r.styleScratch) != len(dst) {
			r.styleScratch = make([]byte, len(dst))
		}
		copy(r.styleScratch, f.scene.SkyMask)
		f.styleMask = r.styleScratch
	}
	if scene.GroundOwners == nil {
		clear(f.scene.SkyMask)
	}
	f.walk()
	if scene.GroundOwners != nil {
		for i, v := range f.scene.SkyMask {
			scene.GroundOwners[i] = byte(SurfaceOwner(v).MaterialOwner())
		}
	}
	if scene.SkyMask != nil {
		copy(scene.SkyMask, f.styleMask)
		for i, v := range f.scene.SkyMask {
			// New proved recipes use exact canonical coverage. Existing accepted
			// recipes keep their established display footprint without falsifying the
			// separate source-owner plane.
			if v == byte(SurfaceFallback84) || v == 0x5c || v == 0x59 || (v >= byte(brownPadWriteSpan) && v < byte(ochrePadWriteSpan)) {
				scene.SkyMask[i] = v
			}
		}
	}
	for i, v := range scene.SkyMask {
		if v == 1 {
			scene.SkyMask[i] = 0
		}
	}
	if err := scene.Environment.applyPixels(dst, r.Assets.WeatherRemap, scene.view().Bounds.Bottom); err != nil {
		return f.stats, err
	}
	if scene.GroundOwners != nil {
		var clearOwner [256]byte
		if err := scene.Environment.applyPixels(scene.GroundOwners, clearOwner[:], scene.view().Bounds.Bottom); err != nil {
			return f.stats, err
		}
		for i, v := range scene.GroundOwners {
			if v == byte(SurfaceOpaque) {
				scene.GroundOwners[i] = 0
			}
		}
	}
	if scene.SkyMask != nil {
		// Weather is a later indexed destination writer. Even a same-family
		// remap owns its pixels; presentation blending must not erase its dots.
		// Reuse that exact pure destination walker with an all-clear owner map;
		// this neither advances weather/noise nor touches canonical pixels.
		var clearOwner [256]byte
		if err := scene.Environment.applyPixels(scene.SkyMask, clearOwner[:], scene.view().Bounds.Bottom); err != nil {
			return f.stats, err
		}
	}
	if f.stats.ArithmeticRejected > 0 {
		f.stats.status(fmt.Sprintf("%d primitives exceed the bounded source projection/clipping arithmetic domain.", f.stats.ArithmeticRejected))
	}
	return f.stats, nil
}

func (f *frame) thresholds() (near, outline, limit int16) {
	near, outline, limit = f.scene.NearReference, f.scene.OutlineThreshold, f.scene.ExtentLimit
	baseNear, baseOutline, baseLimit := int16(0x1000), int16(0x1800), int16(0x2000)
	// Section0:00c1..00ee selects literal threshold triples from SETUP bit04.
	if f.scene.GraphicsOptions&4 != 0 {
		baseNear, baseOutline, baseLimit = 0x2000, 0x2800, 0x4000
	}
	if near == 0 {
		near = baseNear
	}
	if outline == 0 {
		outline = baseOutline
	}
	if limit == 0 {
		limit = baseLimit
	}
	return
}

func absWord(v int16) uint16 {
	if v < 0 {
		return uint16(-v)
	}
	return uint16(v)
}

func (f *frame) culled(g *world.Geometry, translation Vector, cell bool, center bool) bool {
	_, _, limit := f.thresholds()
	extent := g.Extent
	a := uint16(int16(translation[2]>>16) + extent)
	if a >= uint16(limit) {
		return true
	}
	x, y := absWord(int16(translation[0]>>16)), absWord(int16(translation[1]>>16))
	if cell {
		if !center {
			a = uint16(int16(a) >> 1)
		}
		return int16(a) < int16(x) || int16(a) < int16(y)
	}
	return int16(a) < int16(x) || int16(a)>>1 < int16(y)
}

func (f *frame) walk() {
	w := f.r.Assets.Worlds[f.scene.World]
	base := CellIndex(f.scene.Camera.H0, f.scene.Camera.H1)
	registered := map[int][]Instance{}
	for _, in := range f.scene.Instances {
		cell, _, ok := Register(f.scene.Camera, in.Pose)
		if ok {
			registered[cell] = append(registered[cell], in)
		}
	}
	order := innerCells
	if int16(f.scene.Camera.Height) > 512 {
		order = append(append([]cellOffset(nil), outerCells...), innerCells...)
	}
	for _, off := range order {
		index := base + off.x - 64*off.z
		// Out-of-world cells are reported rather than fabricated with wrapping.
		if index < 0 || index >= 4096 || base%64+off.x < 0 || base%64+off.x >= 64 {
			f.stats.status("World-edge neighbor lies outside the prepared4096-cell grid.")
			continue
		}
		cellID := int(w.Grid[index])
		if cellID >= len(w.Cells) {
			f.stats.status("Invalid world cell reference.")
			continue
		}
		cell := w.Cells[cellID]
		g := f.r.Assets.Geometries[cell.Geometry]
		if g == nil {
			f.stats.status("Missing cell geometry " + cell.Geometry)
			continue
		}
		cx := int16(int(f.scene.Camera.H0&8191) - 4096)
		cz := int16(int(f.scene.Camera.H1&8191) - 4096)
		rel := world.Vertex{int16(off.x*8192) - cx, -int16(f.scene.Camera.Height), int16(off.z*8192) - cz}
		trans := Transform(rel, f.matrix)
		center := off.x == 0 && off.z == 0
		cellCulled := false
		if f.fp != nil {
			cellCulled = f.floatCulled(g, f.floatTranslation(rel, ""), true, center)
		} else {
			cellCulled = f.culled(g, trans, true, center)
		}
		if cellCulled {
			f.stats.Rejected++
			continue
		}
		f.stats.Cells++
		f.edges = map[byte]screenEdge{}
		buckets := map[[2]int][]drawItem{}
		for _, in := range registered[index] {
			_, pos, _ := Register(f.scene.Camera, in.Pose)
			key := insertionLeaf(g, pos)
			var model *world.Geometry
			if in.Impact {
				model = &world.Geometry{ID: in.ID, Root: 255}
			} else if in.GeometryID != "" {
				model = f.r.Assets.Geometries[in.GeometryID]
			} else {
				model = f.r.Assets.Geometries[f.r.Assets.Models[in.ModelID]]
			}
			if model == nil {
				f.stats.status("Missing model geometry")
				continue
			}
			if in.MutableModel != nil {
				var err error
				model, err = in.mutableGeometry(model)
				if err != nil {
					f.stats.status("Invalid instance model: " + err.Error())
					continue
				}
			}
			if c := f.r.Assets.Combat; c != nil && (in.GeometryID == c.Templates[0] || in.Explosion) {
				copyModel := *model
				copyModel.Vertices = append([]world.Vertex(nil), model.Vertices...)
				if in.GeometryID == c.Templates[0] && len(copyModel.Vertices) > 5 {
					copyModel.Vertices[5][2] = c.PhaseLengths[f.scene.CombatPhase&3]
				}
				if in.Explosion {
					copy(copyModel.Vertices, f.scene.ExplosionVertices)
				}
				model = &copyModel
			}
			if item, ok := f.placedItem(model, rel, pos, in.Angles, in.oriented(), in.Callback, in.ID); ok {
				item.graphChild = in.GraphChild
				item.impact = in.Impact
				buckets[key] = append(buckets[key], item)
				f.stats.Instances++
			}
		}
		view := world.Vertex{-rel[0], -rel[1], -rel[2]}
		var verts []Vector
		if f.fp != nil {
			f.fp.cell = f.floatVertices(f.fp.cell, g, "", f.floatView, f.floatTranslation(rel, ""))
			f.floatPoints = f.fp.cell
		} else {
			verts = f.vertices(g, f.matrix, trans)
		}
		var walk func(int, int, int, int)
		walk = func(node, parent, side, depth int) {
			if depth > len(g.Planes)+1 {
				f.stats.status("Cyclic prepared plane graph rejected.")
				return
			}
			if node == 255 || node < 0 {
				key := [2]int{parent, side}
				queue := append([]drawItem(nil), buckets[key]...)
				delete(buckets, key)
				for _, a := range cell.Attachments {
					if a.Plane != parent || a.Side != side {
						continue
					}
					for _, p := range a.Placements {
						geom := f.r.Assets.Geometries[p.Geometry]
						if geom == nil {
							f.stats.status("Missing scenery geometry " + p.Geometry)
							continue
						}
						relative := world.Vertex{rel[0] + p.Position[0], rel[1] + p.Position[1], rel[2] + p.Position[2]}
						near, _, _ := f.thresholds()
						rangeMetric := (absWord(relative[0]) >> 1) + (absWord(relative[2]) >> 1)
						rangeMetric >>= f.view.DepthScale
						if rangeMetric >= uint16(near) {
							if p.DistantGeometry == "" {
								continue
							}
							geom = f.r.Assets.Geometries[p.DistantGeometry]
							if geom == nil {
								f.stats.status("Unavailable distant scenery reference")
								continue
							}
						}
						if item, ok := f.placedItem(geom, rel, p.Position, [3]uint16{}, false, 0, p.ID); ok {
							queue = append(queue, item)
							f.stats.Placements++
						}
					}
				}
				for _, item := range sourceQueueOrder(queue) {
					if f.visit != nil {
						f.visit(item)
					} else {
						f.drawItem(item)
					}
				}
				return
			}
			if node >= len(g.Planes) || node >= len(g.Children) {
				f.stats.status("Invalid prepared cell graph reference")
				return
			}
			negative := planeSide(g.Planes[node], view) < 0
			if f.fp != nil {
				q := f.scene.CameraFraction.Position
				negative = floatPlaneNegative(g.Planes[node], floatVertex{float64(view[0]) + q[0], float64(view[1]) + q[1], float64(view[2]) + q[2]})
			}
			first, last := 1, 0
			if negative {
				first, last = 0, 1
			}
			walk(g.Children[node][first], node, first, depth+1)
			if f.visit == nil && node < len(g.PlaneGroups) {
				f.geometryID = g.ID
				f.group(g.PlaneGroups[node], verts, negative, false, true)
			}
			walk(g.Children[node][last], node, last, depth+1)
		}
		if len(g.Children) > 0 {
			walk(g.Root, -1, 0, 0)
		} else {
			for _, item := range buckets[[2]int{-1, 0}] {
				if f.visit != nil {
					f.visit(item)
				} else {
					f.drawItem(item)
				}
			}
			if f.visit == nil {
				for _, group := range g.DirectGroups {
					f.geometryID = g.ID
					f.group(group, verts, false, false, true)
				}
			}
		}
	}
}

func sourceQueueOrder(queue []drawItem) []drawItem {
	result := make([]drawItem, 0, len(queue))
	used := make([]bool, len(queue))
	for len(result) < len(queue) {
		pick := -1
		for i, item := range queue {
			if used[i] {
				continue
			}
			// 3038's one-plane/direct-count-one shortcut wins immediately;
			// it is not a global depth sort, even among these queued children.
			g := item.geometry
			if len(g.Planes) == 1 || len(g.Children) == 0 && len(g.DirectGroups) == 1 {
				pick = i
				break
			}
			if pick < 0 || item.metric > queue[pick].metric {
				pick = i
			}
		}
		if pick < 0 {
			break
		}
		used[pick] = true
		result = append(result, queue[pick])
	}
	return result
}

func insertionLeaf(g *world.Geometry, local world.Vertex) [2]int {
	node, parent, side := g.Root, -1, 0
	for steps := 0; node != 255 && node >= 0 && steps <= len(g.Planes); steps++ {
		if node >= len(g.Children) || node >= len(g.Planes) {
			return [2]int{-1, 0}
		}
		parent = node
		side = 0
		if planeSide(g.Planes[node], local) < 0 {
			side = 1
		}
		node = g.Children[node][side]
	}
	return [2]int{parent, side}
}

func (f *frame) item(g *world.Geometry, relative world.Vertex, angles [3]uint16, oriented bool, callback uint8, id string) (drawItem, bool) {
	if g.EmbeddedPreparation != world.EmbeddedOrdinary {
		// Descriptor312C has near node+8 and zero far node+10. Resident
		// 6794..67C0 uses this position metric before selecting its callback.
		near, _, _ := f.thresholds()
		rangeMetric := (absWord(relative[0]) >> 1) + (absWord(relative[2]) >> 1)
		rangeMetric >>= f.view.DepthScale
		if rangeMetric >= uint16(near) {
			f.stats.Rejected++
			return drawItem{}, false
		}
	}
	translation := Transform(relative, f.matrix)
	if f.culled(g, translation, false, false) {
		f.stats.Rejected++
		return drawItem{}, false
	}
	m := f.matrix
	localView := world.Vertex{-relative[0], -relative[1], -relative[2]}
	if oriented {
		rotation := transpose(Rotation(angles, f.r.Assets.Sine, false))
		// 163c uses 14e6's contiguous row dot products for the local
		// plane-test camera; vertex transformation uses matrix columns.
		v := Transform(localView, transpose(rotation))
		for i := range localView {
			localView[i] = int16(v[i] >> 14)
		}
		m = compose(rotation, f.matrix)
	}
	metric := int16(absWord(int16(translation[0]>>16))) + g.Extent + int16(translation[2]>>16) - int16(translation[1]>>16)
	_, outline, _ := f.thresholds()
	return drawItem{geometry: g, relative: relative, localView: localView, matrix: m, translation: translation, metric: metric, outline: metric >= outline, callback: callback, id: id}, true
}

func (f *frame) drawItem(item drawItem) {
	if item.impact {
		if f.fp != nil {
			v := f.fp.placements[item.floatIndex].translation
			if v[2] > 16 {
				f.paintImpact(floatPixel(f.projectFloat(v, 0)), int(v[2]))
			}
		} else {
			f.drawImpact(item.translation)
		}
		return
	}
	priorGeometry, priorCallback := f.geometryID, f.compactCallback
	f.geometryID, f.compactCallback = item.geometry.ID, item.geometry.CompactCallback
	defer func() { f.geometryID, f.compactCallback = priorGeometry, priorCallback }()
	// Source cache identity belongs to one transformed object, never to the
	// whole scene or to the immutable shared model.
	cellEdges := f.edges
	f.edges = map[byte]screenEdge{}
	defer func() { f.edges = cellEdges }()
	g := item.geometry
	child := func(node, side int) int {
		if edge := item.graphChild; edge != nil && edge.Node == node && edge.Side == side {
			return edge.Child
		}
		return g.Children[node][side]
	}
	var verts []Vector
	if f.fp != nil {
		placement := f.fp.placements[item.floatIndex]
		f.fp.model = f.floatVertices(f.fp.model, g, item.id, placement.matrix, placement.translation)
		f.floatPoints = f.fp.model
		defer func() { f.floatPoints = f.fp.cell }()
	} else {
		verts = f.verticesForInstance(g, item.matrix, item.translation, item.id)
	}
	var draw func(int, int)
	draw = func(node, depth int) {
		if node == 255 || node < 0 {
			return
		}
		if node >= len(g.Children) || node >= len(g.Planes) || depth > len(g.Planes) {
			f.stats.status("Invalid prepared model graph")
			return
		}
		negative := planeSide(g.Planes[node], item.localView) < 0
		if f.fp != nil {
			negative = floatPlaneNegative(g.Planes[node], f.fp.placements[item.floatIndex].localView)
		}
		first, last := 1, 0
		if negative {
			first, last = 0, 1
		}
		draw(child(node, first), depth+1)
		if node < len(g.PlaneGroups) {
			f.group(g.PlaneGroups[node], verts, negative, item.outline, false)
		}
		draw(child(node, last), depth+1)
	}
	if len(g.Children) > 0 {
		draw(g.Root, 0)
	} else {
		for _, group := range g.DirectGroups {
			f.group(group, verts, false, item.outline, false)
		}
	}
}

func (f *frame) vertices(g *world.Geometry, m Matrix, translation Vector) []Vector {
	return f.verticesForInstance(g, m, translation, "")
}

func (f *frame) vertexStream(g *world.Geometry, id string) []world.Vertex {
	base := g.Vertices
	if f.state != nil && f.state.streams[g.ID] != nil {
		base = f.state.streams[g.ID]
	}
	if f.state != nil && f.state.instanceVertices[id] != nil {
		base = f.state.instanceVertices[id]
	}
	return base
}

func (f *frame) verticesForInstance(g *world.Geometry, m Matrix, translation Vector, id string) []Vector {
	if points, ok := playerDecoyVertices(g, m); ok {
		// The embedded 0ECC/13F7 callbacks are private typed preparations,
		// not members of the world compact-family cache. Their origin is the
		// position-only descriptor312C argument; add translation exactly once.
		for i := range points {
			for j := range points[i] {
				points[i][j] += translation[j]
			}
		}
		return points
	}
	if g.CompactCallback > 0 {
		if cached, ok := f.compact[g.CompactCallback]; ok {
			out := make([]Vector, len(cached))
			for i, p := range cached {
				for j := range p {
					out[i][j] = p[j] + translation[j]
				}
			}
			return out
		}
	}
	base := f.vertexStream(g, id)
	var points []Vector
	if g.Generator != nil {
		// Encountered compact command3 is compiled into its semantic shifts.
		// The fourth corner is NOT(U+V), not arithmetic negation.
		s := g.Generator.Shifts
		peak, u, v := Vector{}, Vector{}, Vector{}
		for j := 0; j < 3; j++ {
			peak[j] = int32(m[1][j]) << uint(s[0])
			u[j] = int32(m[0][j]) << uint(s[1])
			v[j] = int32(m[2][j]) << uint(s[2])
		}
		points = append(points, peak)
		for _, kind := range []int{0, 1, 2, 3} {
			var p Vector
			for j := range p {
				switch kind {
				case 0:
					p[j] = u[j] + v[j]
				case 1:
					p[j] = u[j] - v[j]
				case 2:
					p[j] = v[j] - u[j]
				case 3:
					p[j] = ^(u[j] + v[j])
				}
			}
			points = append(points, p)
		}
		previous := append([]Vector(nil), points...)
		for _, delta := range g.Generator.ClusterDeltas {
			cluster := make([]Vector, len(previous))
			for i, p := range previous {
				cluster[i] = p
				cluster[i][0] += int32(delta[0]) << 16
				cluster[i][2] += int32(delta[1]) << 16
			}
			points = append(points, cluster...)
			previous = cluster
		}
	} else {
		points = make([]Vector, len(base))
		for i, v := range base {
			if g.CompactCallback == 5 {
				points[i] = Vector{int32(v[0]) << 16, int32(v[1]) << 16, int32(v[2]) << 16}
			} else {
				points[i] = Transform(v, m)
			}
		}
	}
	if g.CompactCallback > 0 {
		f.compact[g.CompactCallback] = append([]Vector(nil), points...)
	}
	for i := range points {
		for j := range points[i] {
			points[i][j] += translation[j]
		}
	}
	return points
}

func (f *frame) group(group world.Group, points []Vector, negative, outline, terrain bool) {
	if negative && !group.TwoSided {
		return
	}
	if group.Primitive != nil {
		previous := f.material
		f.material = f.r.materialFor(group.Primitive, terrain, f.scene.World, f.geometryID)
		f.primitive(*group.Primitive, points, outline, terrain)
		f.material = previous
	}
	for _, child := range group.Children {
		f.group(child, points, false, outline, terrain)
	}
}

func (f *frame) arithmetic(err error, context string) {
	f.stats.ArithmeticRejected++
	if f.stats.ArithmeticRejected <= 3 {
		f.stats.status(fmt.Sprintf("%s: %v", context, err))
	}
}

func (f *frame) primitive(p world.Primitive, vertices []Vector, outline, terrain bool) {
	if f.fp != nil {
		f.floatPrimitive(p, outline)
		return
	}
	// Style admission has its own screen-edge history. Depth clipping and
	// projection depend only on the primitive geometry and can be shared.
	f.verticesScratch = slices.Grow(f.verticesScratch[:0], len(p.Indices))
	points := f.verticesScratch[:len(p.Indices)]
	for i, index := range p.Indices {
		if index < 0 || index >= len(vertices) {
			f.stats.status("Prepared vertex reference outside geometry")
			return
		}
		points[i] = vertices[index]
	}
	f.stats.Primitives++
	depthChanged := false
	for _, v := range points {
		z := int16(v[2] >> 16)
		depthChanged = depthChanged || z < 8 || z > 8192
	}
	if p.Opcode == 1 {
		if len(points) != 1 {
			f.stats.status("Invalid point primitive")
			return
		}
		z := int16(points[0][2] >> 16)
		if z <= 16 {
			return
		}
		pos, err := f.project(points[0], false)
		if err != nil {
			f.arithmetic(err, "Point projection")
			return
		}
		layout := f.scene.Layout()
		if pos.X < 0 || pos.X >= layout.X[300] || pos.Y < layout.Top || pos.Y >= layout.Y[150] {
			return
		}
		radius, err := idiv((int64(p.Radius)*int64(layout.Width))<<8, int64(z)*Width)
		if err != nil {
			f.arithmetic(err, "Point radius")
			return
		}
		if radius < 0 {
			return
		}
		if radius > layout.X[319] {
			f.stats.status("Projected point radius exceeds diagnostic raster domain")
			return
		}
		f.rasterDisc(p, pos, radius, outline)
		return
	}
	line := p.Opcode == 0x82
	if line {
		if len(points) != 2 {
			return
		}
		a, b := high(points[0]), high(points[1])
		if a[2] > 8192 || b[2] > 8192 || a[2] < 8 && b[2] < 8 {
			return
		}
		if a[2] < 8 {
			v, e := depthIntersection(a, b, 8)
			if e != nil {
				f.arithmetic(e, "Line near clipping")
				return
			}
			points[0] = v
		}
		if b[2] < 8 {
			v, e := depthIntersection(b, a, 8)
			if e != nil {
				f.arithmetic(e, "Line near clipping")
				return
			}
			points[1] = v
		}
	} else {
		clipped, err := clipDepthPolygonInto(f.clippedScratch[:0], points, 8, 8192)
		f.clippedScratch = clipped
		if err != nil {
			f.arithmetic(err, "Polygon depth clipping")
			return
		}
		points = clipped
		if len(points) < 3 {
			return
		}
	}
	f.projectedScratch = slices.Grow(f.projectedScratch[:0], len(points))
	projected := f.projectedScratch[:len(points)]
	for i, p := range points {
		if f.view.DepthScale != 0 {
			p[2] >>= f.view.DepthScale
			if p[2]>>16 < 8 {
				p[2] = 8 << 16
			}
		}
		q, err := f.project(p, true)
		if err != nil {
			f.arithmetic(err, fmt.Sprintf("Primitive projection at %v", p))
			return
		}
		projected[i] = q
	}
	f.rasterPrimitive(p, projected, outline, depthChanged)
}

func (f *frame) rasterDisc(p world.Primitive, pos Point, radius int, outline bool) {
	styleWord := uint16(0x0101)
	if byte(p.Colour) == 0 && p.Colour != 0 {
		styleWord = 0x0100
	}
	owner := f.surfaceOwnership(outline)
	if fillDiscOwned(f.dst, pos, radius, p.Colour, f.view.Bounds, f.scene.GraphicsOptions, f.scene.MaterialCarrier, owner, f.foliageFeedback(p)) {
		f.stats.SourceMemoryFallbackPrimitives++
		f.stats.status("Source-memory material uses " + SourceMemoryFallbackPolicy + "; exact live pattern unavailable.")
	}
	if f.styleMask != nil {
		fillDisc(f.styleMask, pos, radius, styleWord, f.view.Bounds, f.scene.GraphicsOptions, nil)
	}
}

func (f *frame) rasterPrimitive(p world.Primitive, projected []Point, outline, depthChanged bool) {
	styleWord := uint16(0x0101)
	if byte(p.Colour) == 0 && p.Colour != 0 {
		styleWord = 0x0100
	}
	line := p.Opcode == 0x82
	if line || p.Opcode&0x80 != 0 || outline {
		count := len(projected)
		if line {
			count = 1
		}
		clip := f.edgeClipper(p.Flags, p.Indices)
		var styleClip edgeClipper
		if f.styleMask != nil && !depthChanged {
			styleClip = f.styleClipper(p)
		}
		for i := 0; i < count; i++ {
			if f.styleMask != nil {
				a, b := projected[i], projected[(i+1)%len(projected)]
				ok := true
				if styleClip != nil {
					var e error
					a, b, ok, _, e = styleClip(i, a, b, f.view.Bounds)
					ok = ok && e == nil
				}
				if ok {
					_ = DrawLine(f.styleMask, a, b, byte(styleWord), f.view.Bounds)
				}
			}
			a, b := projected[i], projected[(i+1)%len(projected)]
			var err error
			if !depthChanged {
				var ok bool
				a, b, ok, _, err = clip(i, a, b, f.view.Bounds)
				if !ok && err == nil {
					continue
				}
			}
			if err == nil {
				err = DrawLine(f.dst, a, b, byte(p.Colour), f.view.Bounds)
				if err == nil && f.scene.SkyMask != nil {
					err = DrawLine(f.scene.SkyMask, a, b, byte(SurfaceOpaque), f.view.Bounds)
				}
			}
			if err != nil {
				f.arithmetic(err, "Outline screen clipping")
			}
		}
		return
	}
	var clip edgeClipper
	if !depthChanged {
		clip = f.edgeClipper(p.Flags, p.Indices)
	} else if f.fp == nil {
		f.stats.status("Depth-clipped edges recompute intersections; source shared-edge cache identity across generated vertices remains partial.")
	}
	feedback := f.foliageFeedback(p)
	if f.scene.MaterialCarrier == nil && f.scene.GraphicsOptions&1 != 0 && f.material.role == RaisedPHILGround && f.r.Assets.TerrainFeedback.Ready() {
		feedback = f.r.Assets.TerrainFeedback
		f.stats.NativeTerrainFeedbackPrimitives++
		f.stats.status("Raised PHIL terrain uses " + world.TerrainFeedbackPolicy + "; native approximation, exact live pattern unavailable.")
	}
	owner := f.surfaceOwnership(outline)
	fallback, err := fillPolygonMaterialOwned(f.dst, projected, p.Colour, f.view.Bounds, f.scene.GraphicsOptions, f.scene.MaterialCarrier, clip, feedback, owner)
	if !outline && (f.material.role == EUROFieldTop || fallback) {
		if f.material.owner != 0 {
			styleWord = uint16(f.material.owner) * 0x101
		}
	}
	if f.styleMask != nil {
		var sc edgeClipper
		if !depthChanged {
			sc = f.styleClipper(p)
		}
		if f.material.role == GroundContactPad {
			_ = composePadStyleFootprint(f.styleMask, projected, f.view.Bounds, sc)
		} else if token := fieldSpanToken(byte(styleWord)); token != 0 && f.r.Assets.TerrainFeedback.Ready() {
			_, _ = fillPolygonSpans(f.styleMask, projected, styleWord, f.view.Bounds, f.scene.GraphicsOptions, nil, sc, f.r.Assets.TerrainFeedback, nil, token)
		} else {
			_, _ = fillPolygonMaterialOwned(f.styleMask, projected, styleWord, f.view.Bounds, f.scene.GraphicsOptions, nil, sc, nil, nil)
		}
	}
	if err != nil {
		f.arithmetic(err, "Polygon screen clipping")
	}
	if fallback {
		f.stats.SourceMemoryFallbackPrimitives++
		f.stats.status("Source-memory material uses " + SourceMemoryFallbackPolicy + "; exact live pattern unavailable.")
	}
}

// The conservative style footprint starts with an empty shared-edge cache
// for each primitive. It must never populate canonical edge/preview state.
func (f *frame) styleClipper(p world.Primitive) edgeClipper {
	overlay := frame{scene: f.scene, edges: make(map[byte]screenEdge)}
	return overlay.edgeClipper(p.Flags, p.Indices)
}
