package render3d

import (
	"fmt"
	"gunship2000-assets/game/world"
)

// ModelPreview compiles resident361c's small semantic submission record. Its
// translation words occupy the HIGH halves of the source signed32 vectors; they
// are not ordinary world-unit camera offsets. Geometry IDs remain model IDs.
type ModelPreviewResult struct {
	Stats
	HorizontalClipFlags byte
}

type ModelPreview struct {
	ModelID      int
	MutableModel *world.MutableModel
	GeometryID   string
	Angles       [3]uint16
	Translation  world.Vertex
	ID           string
}

func (r *Renderer) DrawModelPreview(scene Scene, state *VisualState, dst []byte, in ModelPreview) (ModelPreviewResult, error) {
	if len(dst) != scene.TargetPixels() {
		return ModelPreviewResult{}, fmt.Errorf("target preview dimensions invalid")
	}
	if err := r.validateScene(scene); err != nil {
		return ModelPreviewResult{}, err
	}
	if in.GeometryID == "" && (in.ModelID < 0 || in.ModelID >= len(r.Assets.Models)) {
		return ModelPreviewResult{}, fmt.Errorf("target preview model invalid")
	}
	id := in.GeometryID
	if id == "" {
		id = r.Assets.Models[in.ModelID]
	}
	g := r.Assets.Geometries[id]
	if g == nil {
		return ModelPreviewResult{}, fmt.Errorf("target preview geometry unavailable")
	}
	var err error
	g, err = (Instance{ModelID: in.ModelID, GeometryID: in.GeometryID, MutableModel: in.MutableModel}).mutableGeometry(g)
	if err != nil {
		return ModelPreviewResult{}, err
	}
	// Source0d70 chooses identity instead of world0d3e's half-scaled first two
	// columns. The caller sets source depth shift44=2; no camera sphere is needed.
	identity := Matrix{{0x4000, 0, 0}, {0, 0x4000, 0}, {0, 0, 0x4000}}
	flags := byte(0)
	f := frame{previewClipFlags: &flags, r: r, scene: scene, state: state, dst: dst, view: scene.rasterView(), matrix: identity, compact: map[int][]Vector{}}
	if scene.FloatingGeometry {
		f.fp = &r.floatScratch
		f.floatView = floatMatrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	}
	item := drawItem{geometry: g, id: in.ID}
	local := world.Vertex{-in.Translation[0], -in.Translation[1], -in.Translation[2]}
	if f.fp != nil {
		// A panel submission already carries a view-relative translation. Its
		// orientation follows the selected actor, without applying world drift.
		rotation := floatRotation(in.Angles, scene.InstanceFractions[in.ID].Angles, false)
		view := transformFloat(-float64(in.Translation[0]), -float64(in.Translation[1]), -float64(in.Translation[2]), rotation)
		for i := range view {
			view[i] *= 4
		}
		placement := floatPlacement{
			matrix:      floatComposeObject(f.floatView, rotation),
			translation: floatVertex{float64(in.Translation[0]), float64(in.Translation[1]), float64(in.Translation[2])},
			localView:   view,
		}
		f.fp.placements = append(f.fp.placements[:0], placement)
	} else {
		rotation := transpose(Rotation(in.Angles, r.Assets.Sine, false))
		v := Transform(local, transpose(rotation))
		for i := range local {
			local[i] = int16(v[i] >> 14)
		}
		item.localView, item.matrix = local, rotation
		item.translation = Vector{int32(in.Translation[0]) << 16, int32(in.Translation[1]) << 16, int32(in.Translation[2]) << 16}
	}
	f.stats.Instances = 1
	f.drawItem(item)
	return ModelPreviewResult{f.stats, flags}, nil
}
