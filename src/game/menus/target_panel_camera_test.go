package menus

import (
	"math"
	"testing"

	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
)

func TestDisplayTargetPanelTerrainClearance(t *testing.T) {
	// A flat cell at elevation zero. Fractional height matters at the lip.
	assets := &world.Bundle{
		Worlds:     map[string]*world.World{"plain": {Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}},
		Geometries: map[string]*world.Geometry{"": {}},
	}
	for _, floating := range []bool{false, true} {
		scene := render3d.Scene{World: "plain", FloatingGeometry: floating}
		scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: 12000, H1: 11000, Height: 4}, Angles: [3]uint16{28, 99, 0}}
		if floating {
			scene.CameraFraction.Position = [3]float64{.25, .125, -.375}
			scene.CameraFraction.Angles[1] = -.375
		}
		above, residual := scene.Camera, scene.CameraFraction
		if err := clearTargetPanelTerrain(&scene, assets, 0, 146); err != nil || scene.Camera != above || scene.CameraFraction != residual {
			t.Fatal("camera with sufficient clearance changed", floating, err)
		}
		if floating {
			scene.CameraFraction.Position[1] = -.125
		} else {
			scene.Camera.Height = 3
		}
		if err := clearTargetPanelTerrain(&scene, assets, 0, 146); err != nil {
			t.Fatal(err)
		}
		if scene.Camera.Height != 4 || scene.CameraFraction.Position[1] != 0 {
			t.Fatal("camera did not clear terrain", scene.Camera)
		}
		angle := float64(int16(scene.Camera.Angles[0])) + scene.CameraFraction.Angles[0]
		miss := 4 + 292*math.Tan(angle*(2*math.Pi/65536))
		tolerance := .015 // Half an integer angle unit at 292 units.
		if floating {
			tolerance = 1e-10
		}
		if math.Abs(miss) > tolerance {
			t.Fatalf("target is off axis by %g", miss)
		}
		above.Angles[0] = scene.Camera.Angles[0]
		residual.Angles[0], residual.Position[1] = scene.CameraFraction.Angles[0], 0
		if scene.Camera != above || scene.CameraFraction != residual {
			t.Fatal("terrain adjustment changed the orbit or yaw")
		}
	}
}
