package render3d

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/world"
)

func TestSubmarinePreparedRenderHeaderIsPrivateAndDrawPure(t *testing.T) {
	path := os.Getenv("GS2000_TEST_WORLD")
	if path == "" {
		t.Skip("set GS2000_TEST_WORLD to prepared world directory")
	}
	prepared, err := world.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	g := prepared.Geometries[prepared.Models[219]]
	b := tinyBundle()
	b.Models = make([]string, 220)
	b.Models[219] = "sub"
	b.Geometries["sub"] = g
	before := append([]world.Vertex(nil), g.Vertices...)
	children := append([][2]int(nil), g.Children...)
	a, _ := world.NewSubmarineModel(b, 219)
	other, _ := world.NewSubmarineModel(b, 219)
	in := Instance{ID: "sub-one", ModelID: 219, MutableModel: a, Pose: Pose{H0: 200512, H1: 200000}, Oriented: true}
	r, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		scene := Scene{World: "GULF5", RenderScale: scale, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 32}, Angles: [3]uint16{0, 0xc000, 0}}, View: FullView(), Instances: []Instance{in}}
		state := &VisualState{}
		baseline, after := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
		if _, err = r.Draw(scene, state, baseline); err != nil {
			t.Fatal(err)
		}
		// Keep this test independent of the previous scale's logical mutation.
		*a = *other
		a.Vertices = append([]world.Vertex(nil), other.Vertices...)
		a.Children = append([][2]int(nil), other.Children...)
		if _, err = r.Draw(scene, state, baseline); err != nil {
			t.Fatal(err)
		}
		for counter := int16(-159); counter <= -100; counter++ {
			if err = a.SubmergeVisit(counter); err != nil {
				t.Fatal(err)
			}
		}
		logical, _ := json.Marshal(a)
		if _, err = r.Draw(scene, state, after); err != nil || bytes.Equal(baseline, after) {
			t.Fatal("real mutable vertices/header did not affect native pixels", scale, err)
		}
		again, _ := json.Marshal(a)
		if !bytes.Equal(logical, again) || !reflect.DeepEqual(g.Vertices, before) || !reflect.DeepEqual(g.Children, children) || !reflect.DeepEqual(other.Vertices, before) {
			t.Fatal("draw mutated logical state or shared/other instance")
		}
		scene.Instances[0].MutableModel = other
		control := make([]byte, scene.TargetPixels())
		if _, err = r.Draw(scene, state, control); err != nil || !bytes.Equal(baseline, control) {
			t.Fatal("second same-model instance inherited the first dive", err)
		}
		// The small target-MFD path bypasses ordinary scene instances. It must
		// submit the same private source header, including during playback.
		preview := ModelPreview{ID: "sub-mfd", ModelID: 219, Angles: [3]uint16{0, 0x4000, 0}, Translation: world.Vertex{0, 0, 512}, MutableModel: other}
		mfdBefore, mfdAfter := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
		if _, err = r.DrawModelPreview(scene, state, mfdBefore, preview); err != nil {
			t.Fatal(err)
		}
		preview.MutableModel = a
		if _, err = r.DrawModelPreview(scene, state, mfdAfter, preview); err != nil || bytes.Equal(mfdBefore, mfdAfter) {
			t.Fatal("small-model MFD ignored private geometry", scale, err)
		}
		preview.MutableModel = other
		mfdControl := make([]byte, scene.TargetPixels())
		if _, err = r.DrawModelPreview(scene, state, mfdControl, preview); err != nil || !bytes.Equal(mfdBefore, mfdControl) {
			t.Fatal("small-model MFD mutated other/shared instance", scale, err)
		}
		if dir := os.Getenv("GS2000_R23_SUBMARINE_CAPTURE"); dir != "" {
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			palette := make(color.Palette, 256)
			for i := range palette {
				palette[i] = color.RGBA{byte(i), byte(i), byte(i), 255}
			}
			for name, pixels := range map[string][]byte{"before": baseline, "submerging": after, "other-unchanged": control} {
				img := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), palette)
				copy(img.Pix, pixels)
				file, err := os.Create(filepath.Join(dir, name+"-"+string(rune('0'+scale))+"x.png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, img)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	in.MutableModel.ModelID = 1
	if _, err := in.mutableGeometry(g); err == nil {
		t.Fatal("mismatched model allowed")
	}
}
