package menus

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/render3d"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBrownBlendOwnershipAndScale(t *testing.T) {
	palette := make(color.Palette, 256)
	for i := range palette {
		palette[i] = color.RGBA{byte(i), byte(i), byte(i), 255}
	}
	for i, v := range []color.RGBA{{130, 97, 65, 255}, {142, 105, 73, 255}, {150, 121, 85, 255}, {162, 130, 93, 255}} {
		palette[0xb4+i] = v
	}
	for _, scale := range []int{1, 2} {
		width, height := 320*scale, 200*scale
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		indices := bytes.Repeat([]byte{0xb7}, width*height)
		mask := bytes.Repeat([]byte{0xb4}, len(indices))
		original := bytes.Clone(indices)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dst.Set(x, y, palette[0xb7])
			}
		}
		mask[0] = 0
		indices[1] = 31
		blocked := image.Rect(20*scale, 20*scale, 30*scale, 30*scale)
		blendBrownTerrainPresentation(dst, indices, mask, palette, blocked)
		if dst.RGBAAt(0, 0).R != 162 || dst.RGBAAt(1, 0).R != 162 || dst.RGBAAt(20*scale, 20*scale).R != 162 {
			t.Fatal("crossed geometry/UI/blocked boundary")
		}
		for y := 31 * scale; y < 40*scale; y++ {
			for x := 31 * scale; x < 40*scale; x++ {
				v := dst.RGBAAt(x, y)
				if v.R < 144 || v.R > 148 || v.G < 111 || v.G > 115 || v.B < 77 || v.B > 81 {
					t.Fatal("mean/hue outside bounded grain", v)
				}
			}
		}
		indices[1] = 0xb7
		if !bytes.Equal(indices, original) {
			t.Fatal("canonical changed")
		}
	}
}

// Source-prepared world, supplied static cameras; not a naturally reached sortie.
func TestMaterialPreparedViews(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("external current preparation required")
	}
	a, e := assetdata.LoadWorldAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	r, e := render3d.New(a)
	if e != nil {
		t.Fatal(e)
	}
	world := a.Worlds["EURO6"]
	index := -1
	for i, v := range world.Grid {
		if v == 22 && i%64 > 10 && i%64 < 50 && i/64 > 10 && i/64 < 50 {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("verified source cell22 missing")
	}
	x, z := index%64, 63-index/64
	out := os.Getenv("GS2000_P5_MATERIAL_OUTPUT")
	if out != "" {
		if e = os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
	}
	total := 0
	for _, name := range []string{"EURO6", "GULF5", "ANTC2", "PHIL2"} {
		for _, scale := range []int{1, 2} {
			for _, height := range []uint16{128, 512} {
				scene := render3d.Scene{World: name, RenderScale: scale, GraphicsOptions: 3, Camera: render3d.Camera{Pose: render3d.Pose{H0: uint32(x*8192 + 4096), H1: uint32(z*8192 + 4096), Height: height}}, View: render3d.FullView()}
				plain, masked := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
				if _, e = r.Draw(scene, nil, plain); e != nil {
					t.Fatal(e)
				}
				scene.SkyMask = make([]byte, len(masked))
				if _, e = r.Draw(scene, nil, masked); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(plain, masked) {
					t.Fatal("ownership mutated canonical", name, scale, height)
				}
				palette, e := r.Palette(scene, nil)
				if e != nil {
					t.Fatal(e)
				}
				before, e := indexedFrameRGBA(masked, palette)
				if e != nil {
					t.Fatal(e)
				}
				after, e := indexedFrameRGBA(masked, palette)
				if e != nil {
					t.Fatal(e)
				}
				blendBrownTerrainPresentation(after, masked, scene.SkyMask, palette, image.Rectangle{})
				changed := !bytes.Equal(before.Pix, after.Pix)
				owned := 0
				for _, v := range scene.SkyMask {
					if render3d.SurfaceOwner(v).MaterialOwner() == render3d.SurfaceBrown {
						owned++
					}
				}
				if name != "EURO6" && changed {
					t.Fatal("unrelated world changed", name)
				}
				total += owned
				t.Logf("source world=%s tileIndex=%d pose=%+v scale%d ownerPixels%d changed%v", name, index, scene.Camera.Pose, scale, owned, changed)
				if out != "" && name == "EURO6" {
					for _, q := range []struct {
						suffix string
						img    image.Image
					}{{"before", before}, {"after", after}} {
						file := filepath.Join(out, name+"-"+q.suffix+"-"+string(rune('0'+scale))+"-"+map[uint16]string{128: "near", 512: "distant"}[height]+".png")
						f, err := os.Create(file)
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(f, q.img)
						f.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("source brown material not reached")
	}
}

func TestTargetMaterialPassPreservesCanonicalAndCaption(t *testing.T) {
	for _, scale := range []int{1, 2} {
		c := mastFixture(t, 2, "land")
		jFinishFade(t, c)
		p := c.Rendering
		p.Scene.World = "EURO6"
		p.Scene.PaletteBank = "DAY"
		p.Scene.Environment = nil
		p.PaletteState = nil
		p.Scene.RenderScale = scale
		p.Scene.GraphicsOptions |= 9
		// Supplied presentation-only pose over the independently inventoried tile718;
		// this does not establish a naturally flown mission or original runtime parity.
		h0, h1 := uint32(118784), uint32(430080)
		p.Scene.Camera = render3d.Camera{Pose: render3d.Pose{H0: h0, H1: h1, Height: 128}}
		row := -1
		for i, e := range p.Initialized.Entities {
			if i > 0 && e.ModelID > 0 {
				for _, candidate := range p.instances {
					if candidate.ID == e.ID {
						row = i
						break
					}
				}
				if row >= 0 {
					break
				}
			}
		}
		if row < 0 {
			t.Fatal("fixture actual target missing")
		}
		p.Flight.Avionics.Target = uint16(row + 1)
		p.Flight.Avionics.TargetPanelZoom = 512
		target := &p.Initialized.Entities[row]
		target.Pose.H0, target.Pose.H1, target.Pose.Height = h0, h1, 128
		p.Flight.State.Pose.H0, p.Flight.State.Pose.H1, p.Flight.State.Pose.Height = h0, h1-4096, 128
		for i := range p.instances {
			if p.instances[i].ID == target.ID {
				p.instances[i].Pose.H0, p.instances[i].Pose.H1, p.instances[i].Pose.Height = h0, h1, 128
			}
		}
		in := p.Cockpit
		in.CameraMode = 0
		in.Arrangement = 0
		in.PrimaryMFD = 5
		in.Damage[0] &^= 0x20
		in.SystemFlags |= 1
		flightBefore, _ := json.Marshal(p.Flight.State)
		plain, masked := make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
		p.collectMaterialMask = false
		if e := p.drawTargetMFD(plain, in, c.Office.Assets.Cockpit); e != nil {
			t.Fatal(e)
		}
		p.collectMaterialMask = true
		if e := p.drawTargetMFD(masked, in, c.Office.Assets.Cockpit); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(plain, masked) {
			t.Fatal("MFD canonical changed with ownership capture")
		}
		count := 0
		for at, v := range p.targetBrownMask {
			if render3d.SurfaceOwner(v).MaterialOwner() == render3d.SurfaceBrown {
				count++
				if masked[at] < 0xb4 || masked[at] > 0xb7 {
					t.Fatal("MFD owner retained a nonbrown final destination")
				}
			}
		}
		if count == 0 {
			t.Fatal("actual full-scene MFD material was not reached", scale, "mask", len(p.targetBrownMask), "target", target.ID, "instances", len(p.instances), "aircraft", in.AircraftID)
		}
		t.Logf("actual clipped full-scene target MFD scale%d brown destinations%d", scale, count)
		p.Cockpit = in
		p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
		p.presentation.mode = "stepped"
		p.resetPresentation()
		p.Dirty = true
		if e := p.draw(c.Office.Assets.Cockpit); e != nil {
			t.Fatal(e)
		}
		merged := 0
		for at, v := range p.targetBrownMask {
			if render3d.SurfaceOwner(v).MaterialOwner() == render3d.SurfaceBrown && p.Frame[at] >= 0xb4 && p.Frame[at] <= 0xb7 {
				if p.skyMask[at] != v {
					t.Fatal("MFD owner lost during overlay composition")
				}
				merged++
			}
		}
		if merged == 0 {
			t.Fatal("MFD ownership not propagated to normal presentation")
		}
		canonical := bytes.Clone(p.Frame)
		rgba, e := c.RenderRGBA(false)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(canonical, p.Frame) {
			t.Fatal("RGBA material treatment altered canonical frame")
		}
		altered := 0
		for at, v := range p.targetBrownMask {
			if render3d.SurfaceOwner(v).MaterialOwner() == render3d.SurfaceBrown && canonical[at] >= 0xb4 && canonical[at] <= 0xb7 {
				r, g, b, _ := c.CurrentPalette()[canonical[at]].RGBA()
				actual := rgba.RGBAAt(at%(320*scale), at/(320*scale))
				if actual.R != byte(r>>8) || actual.G != byte(g>>8) || actual.B != byte(b>>8) {
					altered++
				}
			}
		}
		if altered == 0 {
			t.Fatal("normal RGBA route did not apply MFD material treatment")
		}
		// Both field modes consume the surviving main/MFD owner at the final
		// display resolution. Changing the host preference cannot change the index.
		experimental := bytes.Clone(rgba.Pix)
		c.SimplifiedFields = true
		checker, err := c.RenderRGBA(false)
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for at, v := range p.targetBrownMask {
			if render3d.SurfaceOwner(v).MaterialOwner() == render3d.SurfaceBrown && p.skyMask[at] == v && !bytes.Equal(experimental[at*4:at*4+4], checker.Pix[at*4:at*4+4]) {
				changed++
			}
		}
		if changed == 0 || !bytes.Equal(canonical, p.Frame) {
			t.Fatal("selected field treatment did not reach surviving target MFD samples", changed)
		}
		c.SimplifiedFields = false
		t.Logf("normal cockpit composition scale%d mergedMFD%d actualRGBAdifferences%d", scale, merged, altered)
		if out := os.Getenv("GS2000_P5_MATERIAL_OUTPUT"); out != "" {
			before, e := indexedFrameRGBA(p.Frame, c.CurrentPalette())
			if e != nil {
				t.Fatal(e)
			}
			for _, q := range []struct {
				suffix string
				img    image.Image
			}{{"before", before}, {"after", rgba}} {
				f, e := os.Create(filepath.Join(out, "cockpit-"+q.suffix+"-"+string(rune('0'+scale))+".png"))
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(f, q.img)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
		}

		captured := bytes.Clone(p.targetBrownMask)
		var clip byte
		if e := p.sourceTargetClip(c.Office.Assets.Cockpit, &clip); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(captured, p.targetBrownMask) {
			t.Fatal("canonical clip probe mutated presentation mask")
		}
		flightAfter, _ := json.Marshal(p.Flight.State)
		if !bytes.Equal(flightBefore, flightAfter) {
			t.Fatal("presentation mask changed flight state")
		}
		if out := os.Getenv("GS2000_P5_MATERIAL_OUTPUT"); out != "" {
			palette, e := p.Renderer.Palette(p.Scene, &p.Visual)
			if e != nil {
				t.Fatal(e)
			}
			before, e := indexedFrameRGBA(masked, palette)
			if e != nil {
				t.Fatal(e)
			}
			after, e := indexedFrameRGBA(masked, palette)
			if e != nil {
				t.Fatal(e)
			}
			blendBrownTerrainPresentation(after, masked, p.targetBrownMask, palette, image.Rectangle{})
			for _, q := range []struct {
				suffix string
				img    image.Image
			}{{"before", before}, {"after", after}} {
				f, e := os.Create(filepath.Join(out, "MFD-"+q.suffix+"-"+string(rune('0'+scale))+".png"))
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(f, q.img)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
		}
	}
}

func TestBrownUniformPaletteDoesNotInventNightGrain(t *testing.T) {
	palette := make(color.Palette, 256)
	for i := range palette {
		palette[i] = color.RGBA{0, 0, 0, 255}
	}
	dst := image.NewRGBA(image.Rect(0, 0, 320, 200))
	indices := bytes.Repeat([]byte{0xb7}, 64000)
	mask := bytes.Repeat([]byte{0xb4}, 64000)
	blendBrownTerrainPresentation(dst, indices, mask, palette, image.Rectangle{})
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			if dst.RGBAAt(x, y) != (color.RGBA{0, 0, 0, 255}) {
				t.Fatal("invented night luminance")
			}
		}
	}
}

func BenchmarkBrownMaterialPresentation(b *testing.B) {
	for _, c := range []struct {
		name  string
		scale int
	}{{"320x200", 1}, {"640x400", 2}} {
		b.Run(c.name, func(b *testing.B) {
			dst := image.NewRGBA(image.Rect(0, 0, 320*c.scale, 200*c.scale))
			indices := bytes.Repeat([]byte{0xb7}, 64000*c.scale*c.scale)
			mask := bytes.Repeat([]byte{0xb4}, len(indices))
			pal := make(color.Palette, 256)
			for i := range pal {
				pal[i] = color.RGBA{byte(i), byte(i), byte(i), 255}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				blendBrownTerrainPresentation(dst, indices, mask, pal, image.Rectangle{})
			}
		})
	}
}
