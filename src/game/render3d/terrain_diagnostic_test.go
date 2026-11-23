package render3d

import (
	"gunship2000-assets/game/world"
	"testing"
)

func TestSourceTerrainCoverageDiagnostic(t *testing.T) {
	b := originalBundle(t, "GULF5")
	scene := sourceTrainingScene(t)
	w := b.Worlds[scene.World]
	index := CellIndex(scene.Camera.H0, scene.Camera.H1)
	cell := w.Cells[int(w.Grid[index])]
	g := b.Geometries[cell.Geometry]
	rel := world.Vertex{-int16(int(scene.Camera.H0&8191) - 4096), -int16(scene.Camera.Height), -int16(int(scene.Camera.H1&8191) - 4096)}
	f := frame{r: &Renderer{Assets: b}, scene: scene, view: scene.view(), matrix: Rotation(scene.Camera.Angles, b.Sine, true), compact: map[int][]Vector{}}
	points := f.vertices(g, f.matrix, Transform(rel, f.matrix))
	t.Logf("owncell index%d ID%d geom%s rel%v vertices%v planes%v children%v root%d", index, w.Grid[index], g.ID, rel, g.Vertices, g.Planes, g.Children, g.Root)
	var inspect func(world.Group)
	inspect = func(group world.Group) {
		if p := group.Primitive; p != nil {
			v := make([]Vector, len(p.Indices))
			for i, j := range p.Indices {
				v[i] = points[j]
			}
			c, e := clipDepthPolygon(v, 8, 8192)
			if e != nil {
				t.Logf("depth %v", e)
				return
			}
			projected := []Point{}
			for _, q := range c {
				x, e := project(q, f.view.Center)
				if e != nil {
					t.Logf("project %v", e)
					return
				}
				projected = append(projected, x)
			}
			spans, e := polygonSpans(projected, f.view.Bounds)
			t.Logf("prim%02x colour%04x refs%v raw%v clipped%v projected%v spans140=%d..%d 199=%d..%d err%v", p.Opcode, p.Colour, p.Indices, v, c, projected, spans.left[140], spans.right[140], spans.left[199], spans.right[199], e)
		}
		for _, c := range group.Children {
			inspect(c)
		}
	}
	for _, group := range g.PlaneGroups {
		inspect(group)
	}
	for _, group := range g.DirectGroups {
		inspect(group)
	}
}
