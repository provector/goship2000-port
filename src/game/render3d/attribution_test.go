package render3d

import (
	"encoding/json"
	"gunship2000-assets/game/world"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Optional per-producer attribution of a captured native scene input. It uses
// actual traversal/commands; terrain is intentionally omitted in isolated masks.
func TestPrimitiveAttribution(t *testing.T) {
	input, output, assets := os.Getenv("GS2000_I1_SCENE_INPUT"), os.Getenv("GS2000_I1_ATTRIBUTION"), os.Getenv("GS2000_TEST_WORLD")
	if input == "" || output == "" || assets == "" {
		t.Skip("explicit source-scene input and new output required")
	}
	if err := os.Mkdir(output, 0755); err != nil {
		t.Fatal(err)
	}
	b, err := world.Load(assets)
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var scene Scene
	if err = json.Unmarshal(raw, &scene); err != nil {
		t.Fatal(err)
	}
	state := &VisualState{}
	if err = r.Advance(scene, state, 0); err != nil {
		t.Fatal(err)
	}
	palette, err := r.Palette(scene, state)
	if err != nil {
		t.Fatal(err)
	}
	type attribution struct {
		ID, Geometry         string
		Relative             world.Vertex
		LocalView            world.Vertex
		Pixels, AboveHorizon int
		Bounds               Rect
	}
	records := []attribution{}
	f := frame{r: r, scene: scene, state: state, view: scene.view(), matrix: Rotation(scene.Camera.Angles, b.Sine, true), compact: map[int][]Vector{}}
	f.visit = func(item drawItem) {
		dst := make([]byte, 64000)
		for i := range dst {
			dst[i] = 0xff
		}
		f.dst = dst
		f.drawItem(item)
		record := attribution{ID: item.id, Geometry: item.geometry.ID, Relative: item.relative, LocalView: item.localView, Bounds: Rect{319, 199, 0, 0}}
		for i, c := range dst {
			if c == 0xff {
				continue
			}
			record.Pixels++
			x, y := i%320, i/320
			if y < scene.View.Center.Y {
				record.AboveHorizon++
			}
			if x < record.Bounds.Left {
				record.Bounds.Left = x
			}
			if x > record.Bounds.Right {
				record.Bounds.Right = x
			}
			if y < record.Bounds.Top {
				record.Bounds.Top = y
			}
			if y > record.Bounds.Bottom {
				record.Bounds.Bottom = y
			}
		}
		records = append(records, record)
		if item.id == "mission-row-0" {
			img := image.NewPaletted(image.Rect(0, 0, 320, 200), palette)
			copy(img.Pix, dst)
			p, e := os.Create(filepath.Join(output, "ownship.png"))
			if e != nil {
				t.Fatal(e)
			}
			if e = png.Encode(p, img); e != nil {
				t.Fatal(e)
			}
			p.Close()
		}
	}
	f.walk()
	raw, err = json.MarshalIndent(records, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "producers.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("attributed%d queued model/scenery producers; source-scene%s", len(records), input)
}
