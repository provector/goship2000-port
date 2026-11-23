package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/render3d"
	"testing"
)

func TestMaterialDrawSchedulesDoNotMutateMission(t *testing.T) {
	c := nResultController(t)
	p := c.Rendering
	// Presentation-only PHIL2 face poses identify the source37 jungle cell.
	// The actual generated Gulf mission and all authoritative positions stay intact.
	original := p.Renderer.Assets.TerrainFeedback
	if !original.Ready() {
		t.Fatal("fresh material metadata required")
	}
	snapshot := func() []byte {
		b, err := json.Marshal(struct {
			Flight  any
			Mission any
			Targets any
		}{p.Flight, p.Initialized, p.ResultTargets})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	before := snapshot()
	for _, scale := range []int{1, 2} {
		for _, draws := range []int{30, 60, 144} {
			for _, candidate := range []bool{false, true} {
				t.Run(fmt.Sprintf("scale%d/draws%d/candidate%t", scale, draws, candidate), func(t *testing.T) {
					if candidate {
						p.Renderer.Assets.TerrainFeedback = original
					} else {
						p.Renderer.Assets.TerrainFeedback = nil
					}
					scene := p.Scene
					scene.World = "PHIL2"
					scene.RenderScale = scale
					scene.View = render3d.FullView()
					scene.GraphicsOptions |= 1
					scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: 346660, H1: 467040, Height: 700}, Angles: [3]uint16{0xf400, 0x800, 0}}
					dst := make([]byte, scene.TargetPixels())
					admitted := 0
					for n := 0; n < draws; n++ {
						scene.Camera.H0 += 12
						scene.Camera.Angles[2] = uint16(int16(n-draws/2) * 48)
						st, err := p.Renderer.Draw(scene, &render3d.VisualState{}, dst)
						if err != nil {
							t.Fatal(err)
						}
						admitted += st.NativeTerrainFeedbackPrimitives
					}
					if candidate && admitted == 0 {
						t.Fatal("material not exercised")
					}
					if !bytes.Equal(before, snapshot()) {
						t.Fatal("extra material/presentation draws mutated authoritative mission/RNG")
					}
				})
			}
		}
	}
	p.Renderer.Assets.TerrainFeedback = original
}
