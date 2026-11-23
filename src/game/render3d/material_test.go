package render3d

import (
	"bytes"
	"testing"
)

func TestBrownTerrainOwnershipLeavesCanonicalAndModels(t *testing.T) {
	for _, scale := range []int{1, 2} {
		b := tinyBundle()
		b.Worlds["EURO6"] = b.Worlds["GULF5"]
		b.Geometries["cell"].PlaneGroups[0].Primitive.Colour = 0xb7
		b.Geometries["cell"].PlaneGroups[0].Primitive.Opcode = 4
		r, e := New(b)
		if e == nil {
			p := b.Geometries["cell"].PlaneGroups[0].Primitive
			r.materials.leaves[p] = materialBinding{role: EUROBrownGround, owner: 0xb4, raw: p.Colour, opcode: p.Opcode, world: "EURO6", root: true, geometry: "cell"}
		}
		if e == nil {
			p := b.Geometries["cell"].PlaneGroups[0].Primitive
			r.materials.leaves[p] = materialBinding{role: EUROBrownGround, owner: 0xb4, raw: p.Colour, opcode: p.Opcode, world: "EURO6", root: true, geometry: "cell"}
		}
		if e != nil {
			t.Fatal(e)
		}
		scene := Scene{World: "EURO6", RenderScale: scale, GraphicsOptions: 1, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, View: FullView()}
		plain, masked := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
		if _, e = r.Draw(scene, nil, plain); e != nil {
			t.Fatal(e)
		}
		scene.SkyMask = make([]byte, len(masked))
		st, e := r.Draw(scene, nil, masked)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(plain, masked) {
			t.Fatal("presentation ownership changed canonical bytes")
		}
		owned := 0
		for at, v := range scene.SkyMask {
			if SurfaceOwner(v).MaterialOwner() == SurfaceBrown {
				owned++
				if masked[at] < 0xb4 || masked[at] > 0xb7 {
					t.Fatal("mask escaped source family")
				}
			}
		}
		if owned == 0 || st.SourceMemoryFallbackPrimitives == 0 {
			t.Fatal("brown terrain not reached", scale, owned, st)
		}
		// Identical material on an OBJECT may not qualify as terrain ownership.
		beforeModel := bytes.Clone(scene.SkyMask)
		if b.Geometries["model"].DirectGroups[0].Primitive.Colour != 0xb7 {
			t.Fatal("same-ink model control missing")
		}
		scene.Instances = []Instance{{ModelID: 0, Pose: Pose{H0: 200000, H1: 200512, Height: 256}}}
		if _, e = r.Draw(scene, nil, masked); e != nil {
			t.Fatal(e)
		}
		removed := 0
		for at, v := range beforeModel {
			if SurfaceOwner(v).MaterialOwner() == SurfaceBrown && scene.SkyMask[at] == 0 {
				removed++
			}
		}
		if removed == 0 {
			t.Fatal("same-brown model did not exercise ownership clearing")
		}
		// Other worlds and a real source-memory carrier never enter the new policy.
		scene.World = "GULF5"
		if _, e = r.Draw(scene, nil, masked); e != nil {
			t.Fatal(e)
		}
		for _, v := range scene.SkyMask {
			if SurfaceOwner(v).MaterialOwner() == SurfaceBrown {
				t.Fatal("policy widened to another world")
			}
		}
		scene.World = "EURO6"
		scene.MaterialCarrier = &Carrier{CS: make([]byte, 64000), DS: make([]byte, 64000)}
		if _, e = r.Draw(scene, nil, masked); e != nil {
			t.Fatal(e)
		}
		for _, v := range scene.SkyMask {
			if SurfaceOwner(v).MaterialOwner() == SurfaceBrown {
				t.Fatal("policy replaced validated original memory")
			}
		}
	}
}

func TestMaterialOwnerPreservesSameFamilyWeatherWrites(t *testing.T) {
	b := tinyBundle()
	b.Worlds["EURO6"] = b.Worlds["GULF5"]
	b.Geometries["cell"].PlaneGroups[0].Primitive.Colour = 0xb7
	b.Geometries["cell"].PlaneGroups[0].Primitive.Opcode = 4
	b.WeatherRemap = make([]byte, 256)
	for i := range b.WeatherRemap {
		b.WeatherRemap[i] = byte(i)
	}
	for i := 0xb4; i <= 0xb7; i++ {
		b.WeatherRemap[i] = 0xb4
	}
	r, e := New(b)
	if e == nil {
		p := b.Geometries["cell"].PlaneGroups[0].Primitive
		r.materials.leaves[p] = materialBinding{role: EUROBrownGround, owner: 0xb4, raw: p.Colour, opcode: p.Opcode, world: "EURO6", root: true, geometry: "cell"}
	}
	if e != nil {
		t.Fatal(e)
	}
	scene := Scene{World: "EURO6", GraphicsOptions: 1, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, View: FullView(), SkyMask: make([]byte, 64000)}
	dst := make([]byte, 64000)
	if _, e = r.Draw(scene, nil, dst); e != nil {
		t.Fatal(e)
	}
	at := -1
	for i, v := range scene.SkyMask {
		if SurfaceOwner(v).MaterialOwner() == SurfaceBrown {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("brown control not reached")
	}
	scene.Environment = &EnvironmentState{noise: []uint16{uint16(at)}}
	if _, e = r.Draw(scene, nil, dst); e != nil {
		t.Fatal(e)
	}
	if dst[at] != 0xb4 || scene.SkyMask[at] != 0 {
		t.Fatal("same-family source weather dot lost its ownership", at, dst[at], scene.SkyMask[at])
	}
}
