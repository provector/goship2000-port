package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/world"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestGroundMaterialMotionAndCanonicalIsolation(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("declared prepared world fixture required")
	}
	a, e := assetdata.LoadWorldAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	r, e := render3d.New(a)
	if e != nil {
		t.Fatal(e)
	}
	out := os.Getenv("GS2000_Q31_GROUND_OUTPUT")
	if out != "" {
		if e = os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
	}
	var worlds []string
	for name := range a.Worlds {
		if strings.HasPrefix(name, "EURO") {
			worlds = append(worlds, name)
		}
	}
	sort.Strings(worlds)
	var records []map[string]any
	for _, site := range []struct {
		family byte
		bank   string
	}{{0x74, "DAY"}, {0x7c, "GREY"}} {
		var chosen render3d.Scene
		var source *world.Geometry
		found := false
		for _, name := range worlds {
			w := a.Worlds[name]
			for at, id := range w.Grid {
				if at%64 < 5 || at%64 > 58 || at/64 < 5 || at/64 > 58 {
					continue
				}
				g := a.Geometries[w.Cells[int(id)].Geometry]
				match := false
				var visit func([]world.Group)
				visit = func(groups []world.Group) {
					for _, v := range groups {
						if p := v.Primitive; p != nil && p.Opcode == 4 && p.Colour>>8 == 0 && byte(p.Colour)&0xfc == site.family {
							match = true
						}
						visit(v.Children)
					}
				}
				visit(g.PlaneGroups)
				visit(g.DirectGroups)
				if !match {
					continue
				}
				for _, heading := range []uint16{0, 0x4000, 0x8000, 0xc000} {
					scene := render3d.Scene{World: name, PaletteBank: site.bank, GraphicsOptions: 3, Camera: render3d.Camera{Pose: render3d.Pose{H0: uint32(at%64*8192 + 4096), H1: uint32((63-at/64)*8192 + 4096), Height: 256}, Angles: [3]uint16{0, heading, 0}}, View: render3d.FullView()}
					scene.SkyMask = make([]byte, scene.TargetPixels())
					pixels := make([]byte, len(scene.SkyMask))
					if _, e = r.Draw(scene, nil, pixels); e != nil {
						t.Fatal(e)
					}
					count := 0
					for _, owner := range scene.SkyMask {
						if family, ok := groundFamily(owner); ok && family == site.family {
							count++
						}
					}
					if count >= 64 {
						chosen = scene
						source = g
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatal("real prepared terrain family not reached", site.family)
		}
		for _, scale := range []int{1, 2} {
			var preceding []byte
			for frame := 0; frame < 8; frame++ {
				scene := chosen
				scene.RenderScale = scale
				scene.SkyMask = make([]byte, scene.TargetPixels())
				scene.Camera.Angles[2] = []uint16{0, 1024, 2048, 0, 0, 0, 0, 0}[frame]
				scene.Camera.Angles[1] += uint16(max(0, frame-2) * 512)
				scene.Camera.H0 += uint32(max(0, frame-3) * 512)
				scene.Camera.H1 += uint32(max(0, frame-3) * 1024)
				plain := make([]byte, scene.TargetPixels())
				unmasked := scene
				unmasked.SkyMask = nil
				if _, e = r.Draw(unmasked, nil, plain); e != nil {
					t.Fatal(e)
				}
				pixels := make([]byte, len(plain))
				if _, e = r.Draw(scene, nil, pixels); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(plain, pixels) {
					t.Fatal("ownership altered canonical raster")
				}
				pal, e := r.Palette(scene, nil)
				if e != nil {
					t.Fatal(e)
				}
				before, e := indexedFrameRGBA(pixels, pal)
				if e != nil {
					t.Fatal(e)
				}
				after, e := indexedFrameRGBA(pixels, pal)
				if e != nil {
					t.Fatal(e)
				}
				blendEuroGroundPresentation(after, pixels, scene.SkyMask, pal, image.Rectangle{})
				count := 0
				for i, owner := range scene.SkyMask {
					family, ok := groundFamily(owner)
					x, y := i%(320*scale), i/(320*scale)
					if ok && family != 0xb4 && pixels[i]&0xfc == family {
						count++
					} else if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
						t.Fatal("crossed material/sky/edge boundary")
					}
				}
				if frame > 0 && bytes.Equal(preceding, scene.SkyMask) {
					t.Fatal("camera path did not move polygon coverage")
				}
				preceding = bytes.Clone(scene.SkyMask)
				name := fmt.Sprintf("family%02x-scale%d-frame%02d", site.family, scale, frame)
				if out != "" {
					for _, v := range []struct {
						suffix string
						img    image.Image
					}{{"before", before}, {"after", after}} {
						f, e := os.Create(filepath.Join(out, name+"-"+v.suffix+".png"))
						if e != nil {
							t.Fatal(e)
						}
						e = png.Encode(f, v.img)
						f.Close()
						if e != nil {
							t.Fatal(e)
						}
					}
				}
				records = append(records, map[string]any{"name": name, "world": scene.World, "camera": scene.Camera, "palette": scene.PaletteBank, "owner_pixels": count, "canonical_sha256": fmt.Sprintf("%x", sha256.Sum256(pixels)), "geometry": source.ID, "geometry_source_sha256": source.SourceHash, "geometry_source_offset": source.SourceOffset, "classification": "Prepared original geometry with supplied deterministic camera path; equivalent material, not Matt's unrecorded historical position"})
			}
		}
	}
	if out != "" {
		b, _ := json.MarshalIndent(records, "", "  ")
		if e = os.WriteFile(filepath.Join(out, "index.json"), append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
