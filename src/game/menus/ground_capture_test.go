package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	a "gunship2000-assets/game/assets"
	"gunship2000-assets/game/render3d"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Portable across the R6/R7 source boundary: actual scene Draw -> Frame ->
// RenderRGBA, no helper-only colour oracle and no aircraft/result injection.
func TestGroundCaptureProductionScenes(t *testing.T) {
	assets, err := a.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	scenes := os.Getenv("GS2000_Q32_GROUND_SCENES")
	if scenes == "" {
		t.Skip("retained scene fixtures required")
	}
	out := os.Getenv("GS2000_R7_CAPTURE")
	if out != "" {
		if e := os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
	}
	r, e := render3d.New(assets.World)
	if e != nil {
		t.Fatal(e)
	}
	var reports []map[string]any
	for _, tc := range []struct{ name, file, world string }{{"pad-euro7", "pad-scene.json", ""}, {"pad-euro6", "pad-scene.json", "EURO6"}, {"field-euro8", "field-scene.json", "EURO8"}, {"brown-no-pad", "pad-scene.json", "EURO6"}, {"phil1-near", "field-scene.json", "PHIL1"}, {"phil2-far", "field-scene.json", "PHIL2"}, {"phil3-translated", "field-scene.json", "PHIL3"}} {
		raw, e := os.ReadFile(filepath.Join(scenes, tc.file))
		if e != nil {
			t.Fatal(e)
		}
		var source render3d.Scene
		if e = json.Unmarshal(raw, &source); e != nil {
			t.Fatal(e)
		}
		if tc.world != "" {
			wanted := byte(22)
			if tc.world == "EURO8" {
				wanted = 23
			}
			w := assets.World.Worlds[tc.world]
			oldX, oldZ := int(source.Camera.H0>>13), int(source.Camera.H1>>13)
			best, dist := -1, int(^uint(0)>>1)
			for at, id := range w.Grid {
				if id != wanted {
					continue
				}
				x, z := at%64, 63-at/64
				d := (x-oldX)*(x-oldX) + (z-oldZ)*(z-oldZ)
				if d < dist {
					best, dist = at, d
				}
			}
			if best < 0 {
				t.Fatal("required actual root cell absent")
			}
			dx, dz := int32((best%64-oldX)*8192), int32((63-best/64-oldZ)*8192)
			source.World = tc.world
			source.Camera.H0 = uint32(int32(source.Camera.H0) + dx)
			source.Camera.H1 = uint32(int32(source.Camera.H1) + dz)
			for i := range source.Instances {
				source.Instances[i].H0 = uint32(int32(source.Instances[i].H0) + dx)
				source.Instances[i].H1 = uint32(int32(source.Instances[i].H1) + dz)
			}
		}
		if tc.name == "phil2-far" {
			source.Camera.Height += 2048
		}
		if tc.name == "phil3-translated" {
			source.Camera.H0 += 512
			source.Camera.H1 += 256
		}
		if tc.name == "brown-no-pad" {
			var keep []render3d.Instance
			for _, in := range source.Instances {
				if in.ModelID != 131 {
					keep = append(keep, in)
				}
			}
			source.Instances = keep
		}
		for _, scale := range []int{1, 2} {
			for motion := 0; motion < 3; motion++ {
				s := source
				s.RenderScale = scale
				s.Camera.H0 += uint32(motion * 192)
				s.Camera.Angles[1] += uint16(motion * 512)
				s.Camera.Angles[2] += uint16(motion * 256)
				plain, pixels := make([]byte, s.TargetPixels()), make([]byte, s.TargetPixels())
				if _, e = r.Draw(s, nil, plain); e != nil {
					t.Fatal(e)
				}
				s.SkyMask = make([]byte, len(pixels))
				if _, e = r.Draw(s, nil, pixels); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(plain, pixels) {
					t.Fatal("owner collection changed canonical pixels")
				}
				pal, e := r.Palette(s, nil)
				if e != nil {
					t.Fatal(e)
				}
				p := &RenderingPreview{Scene: s, Renderer: r, Frame: pixels, Palette: pal, skyMask: s.SkyMask}
				c := &FrontEnd{Screen: ScreenScene, Office: &Office{Assets: assets}, Rendering: p}
				before, _ := json.Marshal(s)
				rgba, e := c.RenderRGBA(false)
				if e != nil {
					t.Fatal(e)
				}
				again, e := c.RenderRGBA(false)
				if e != nil || !bytes.Equal(rgba.Pix, again.Pix) {
					t.Fatal("repeated normal display changed output", e)
				}
				after, _ := json.Marshal(s)
				if !bytes.Equal(before, after) || !bytes.Equal(plain, pixels) {
					t.Fatal("presentation mutated scene or canonical state")
				}
				name := fmt.Sprintf("%s-scale%d-motion%d", tc.name, scale, motion)
				owners := map[string]int{}
				for _, v := range s.SkyMask {
					owners[fmt.Sprintf("%02x", v)]++
				}
				reports = append(reports, map[string]any{"name": name, "source_scene_sha256": fmt.Sprintf("%x", sha256.Sum256(raw)), "scene": s, "palette": s.PaletteBank, "options": s.GraphicsOptions, "owners": owners, "canonical_sha256": fmt.Sprintf("%x", sha256.Sum256(pixels)), "rgba_sha256": fmt.Sprintf("%x", sha256.Sum256(rgba.Pix)), "scope": "retained supplied render scene; world substitutions are labelled fixtures, not a natural sortie/newest incident"})
				if out != "" {
					f, e := os.Create(filepath.Join(out, name+".png"))
					if e != nil {
						t.Fatal(e)
					}
					if e = png.Encode(f, rgba); e != nil {
						t.Fatal(e)
					}
					f.Close()
				}
			}
		}
	}
	if out != "" {
		raw, _ := json.MarshalIndent(reports, "", "  ")
		if e = os.WriteFile(filepath.Join(out, "frames.json"), append(raw, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
