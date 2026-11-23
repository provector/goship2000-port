package menus

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/render3d"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestReportedGroundPoliciesAndCanonicalIsolation(t *testing.T) {
	fixtures := os.Getenv("GS2000_Q32_GROUND_SCENES")
	if fixtures == "" {
		t.Skip("declared retained-screenshot scene fixtures required")
	}
	a, e := assetdata.LoadWorldAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	r, e := render3d.New(a)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, file string
		step       int
		owner      byte
	}{{"green", "field-scene.json", 515, 0x58}, {"pad", "pad-scene.json", 160, 0x5b}} {
		t.Run(tc.name, func(t *testing.T) {
			raw, e := os.ReadFile(filepath.Join(fixtures, tc.file))
			if e != nil {
				t.Fatal(e)
			}
			var scene render3d.Scene
			if e = json.Unmarshal(raw, &scene); e != nil {
				t.Fatal(e)
			}
			for _, scale := range []int{1, 2} {
				scene.RenderScale = scale
				scene.SkyMask = nil
				plain := make([]byte, scene.TargetPixels())
				if _, e = r.Draw(scene, nil, plain); e != nil {
					t.Fatal(e)
				}
				scene.SkyMask = make([]byte, len(plain))
				masked := make([]byte, len(plain))
				if _, e = r.Draw(scene, nil, masked); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(plain, masked) {
					t.Fatal("presentation ownership changed canonical raster", scale)
				}
				palette, e := r.Palette(scene, nil)
				if e != nil {
					t.Fatal(e)
				}
				before, _ := indexedFrameRGBA(masked, palette)
				after, _ := indexedFrameRGBA(masked, palette)
				blendEuroGroundPresentation(after, masked, scene.SkyMask, palette, image.Rectangle{})
				blendEuroFieldPresentation(after, masked, scene.SkyMask, palette, image.Rectangle{})
				blendEuroPadPresentation(after, masked, scene.SkyMask, palette, image.Rectangle{})
				count, edge, marks := 0, 0, 0
				for at, owner := range scene.SkyMask {
					x, y := at%(320*scale), at/(320*scale)
					changed := before.RGBAAt(x, y) != after.RGBAAt(x, y)
					if byte(render3d.SurfaceOwner(owner).MaterialOwner()) == tc.owner {
						count++
					}
					if owner == 0x49 {
						edge++
					}
					if masked[at] == 0x0e {
						marks++
						if changed {
							t.Fatal("landing-pad marking was repainted", scale, x, y)
						}
					}
					admitted := groundOwnerAdmits(owner, masked[at])
					if changed && !admitted {
						t.Fatal("presentation escaped actual winning ground surface", tc.name, scale, x, y, owner, masked[at])
					}
				}
				if count == 0 {
					t.Fatal("reported actual surface never received owner", tc.name, scale)
				}
				if tc.name == "green" && edge == 0 {
					t.Fatal("bright diagonal edge not independently exercised", scale)
				}
				if tc.name == "pad" {
					at := 85*scale*320*scale + 150*scale
					base, written, pad := render3d.SurfaceOwner(scene.SkyMask[at]).Pad()
					if !pad || base != render3d.SurfaceFallback7C || written != (masked[at] == 0xec) || before.RGBAAt(150*scale, 85*scale) == after.RGBAAt(150*scale, 85*scale) || marks == 0 {
						t.Fatal("screen-door pad did not retain underlying terrain treatment", scale, scene.SkyMask[at], marks)
					}
				}
				blocked := image.Rect(100*scale, 50*scale, 180*scale, 100*scale)
				protected, _ := indexedFrameRGBA(masked, palette)
				blendEuroFieldPresentation(protected, masked, scene.SkyMask, palette, blocked)
				for y := blocked.Min.Y; y < blocked.Max.Y; y++ {
					for x := blocked.Min.X; x < blocked.Max.X; x++ {
						if protected.RGBAAt(x, y) != before.RGBAAt(x, y) {
							t.Fatal("UI exclusion changed", scale, x, y)
						}
					}
				}
				t.Log(tc.name, "scale", scale, "owner pixels", count, "edge", edge, "markings", marks)
			}
		})
	}
}
