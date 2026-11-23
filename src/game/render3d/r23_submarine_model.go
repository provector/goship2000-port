package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
)

// mutableGeometry applies a source per-actor submarine header immediately before
// traversal. No private vertices enter the renderer's shared animation stream.
func (in Instance) mutableGeometry(g *world.Geometry) (*world.Geometry, error) {
	if in.MutableModel == nil {
		return g, nil
	}
	if in.MutableModel.ModelID != in.ModelID || in.GeometryID != "" || in.Callback != 0 {
		return nil, fmt.Errorf("instance%s submarine model/header identity differs", in.ID)
	}
	return in.MutableModel.Apply(g)
}
