package render3d

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/world"
	"os"
	"testing"
)

func materialsBundle(t *testing.T) *world.Bundle {
	t.Helper()
	p := os.Getenv("GS2000_TEST_WORLD")
	if p == "" {
		t.Skip("prepared world required")
	}
	b, e := world.Load(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestVerifiedRegistryAndUnknownSources(t *testing.T) {
	b := materialsBundle(t)
	r, e := New(b)
	if e != nil {
		t.Fatal(e)
	}
	roles := map[MaterialRole]int{}
	for _, v := range r.MaterialInventory() {
		roles[v.Role]++
		if len(v.SourceSHA256) != 64 || len(v.SliceSHA256) != 64 || len(v.LayoutSHA256) != 64 || v.Path == "" {
			t.Fatal("incomplete identity", v)
		}
	}
	if roles[RaisedPHILGround] != 1045 || roles[EUROFieldSide] != 4 || roles[EUROFallbackGround] != 3 || roles[EUROBrownGround] != 1 || roles[GroundContactPad] != 1 || roles[EUROFieldTop] != 1 {
		t.Fatal(roles)
	}
	g := b.Geometries["model/131"]
	p := g.PlaneGroups[0].Children[0].Primitive
	if r.materialFor(p, false, "EURO7", "model/131").role != GroundContactPad || r.materialFor(p, false, "EURO7", "model/130").role != "" || r.materialFor(p, true, "EURO7", "model/131").role != "" {
		t.Fatal("actual use not part of admission")
	}
	cell := b.Geometries["world/EURO7/cell/23"].PlaneGroups[0].Children[4].Primitive
	if r.materialFor(cell, false, "EURO7", "world/EURO7/cell/23").role != "" {
		t.Fatal("attached model acquired root role")
	}
	for _, id := range []string{"world/EURO7/cell/29", "world/EURO7/cell/35"} {
		var check func([]world.Group)
		check = func(gs []world.Group) {
			for i := range gs {
				if p := gs[i].Primitive; p != nil && r.materialFor(p, true, "EURO7", id).role != "" {
					t.Fatal("unjustified named variant")
				}
				check(gs[i].Children)
			}
		}
		check(b.Geometries[id].PlaneGroups)
	}
	old := b.Geometries["world/EURO7/cell/23"]
	copy := *old
	copy.SourceHash = "0000000000000000000000000000000000000000000000000000000000000000"
	b.Geometries[old.ID] = &copy
	unknown, e := New(b)
	b.Geometries[old.ID] = old
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range unknown.MaterialInventory() {
		if v.Geometry == old.ID {
			t.Fatal("unknown source guessed")
		}
	}
	t.Log(roles)
}
func TestPadSamplesAndExactWriterOwnership(t *testing.T) {
	for _, scale := range []int{1, 2} {
		for y := 2; y < 6; y++ {
			for left := 1; left < 5; left++ {
				for width := 1; width <= 9; width++ {
					size := Width * Height * scale * scale
					pixels := bytes.Repeat([]byte{0x7f}, size)
					owners := bytes.Repeat([]byte{byte(SurfaceFallback7C)}, size)
					r := left + width - 1
					o := spanOwnership{mask: owners, material: materialBinding{role: GroundContactPad}}
					o.span(pixels, y, left, r, 0xec00)
					fillSpan(pixels, y, left, r, 0xec00, 1, nil)
					// Source SHR count and independently derived first sample, in logical320 coordinates.
					first := left / scale
					if (first+y/scale)&1 != 0 {
						first++
					}
					count := (r/scale - left/scale + 1) / 2
					for x := left; x <= r; x++ {
						at := y*Width*scale + x
						want := false
						for n := 0; n < count; n++ {
							want = want || x/scale == first+2*n
						}
						base, wrote, pad := SurfaceOwner(owners[at]).Pad()
						if !pad || base != SurfaceFallback7C || wrote != want || (pixels[at] == 0xec) != want {
							t.Fatal(scale, y, left, width, x, pixels[at], owners[at], want)
						}
					}
					// Later opaque writes erase even identical-colour ownership; remap no-op does not.
					o.material = materialBinding{}
					o.span(pixels, y, left, r, 0x7f7f)
					fillSpan(pixels, y, left, r, 0x7f7f, 0, nil)
					for x := left; x <= r; x++ {
						if owners[y*Width*scale+x] != byte(SurfaceOpaque) {
							t.Fatal("stale pad owner")
						}
					}
				}
			}
		}
	}
}
func TestModelOnlyAndCarrierControls(t *testing.T) {
	b := materialsBundle(t)
	r, e := New(b)
	if e != nil {
		t.Fatal(e)
	}
	for _, scale := range []int{1, 2} {
		s := Scene{World: "EURO7", RenderScale: scale, View: FullView(), GraphicsOptions: 3}
		pixels := bytes.Repeat([]byte{0x7f}, s.TargetPixels())
		s.SkyMask = make([]byte, len(pixels))
		before, _ := json.Marshal(s)
		if _, e = r.DrawModelPreview(s, nil, pixels, ModelPreview{ModelID: 131, Angles: [3]uint16{0x0800, 0, 0}, Translation: world.Vertex{0, 0, 2048}}); e != nil {
			t.Fatal(e)
		}
		for _, v := range s.SkyMask {
			if SurfaceOwner(v).ApprovedGround() {
				t.Fatal("model-only preview invented ground underlay")
			}
		}
		after, _ := json.Marshal(s)
		if !bytes.Equal(before, after) {
			t.Fatal("preview mutated scene")
		}
	}
	// Existing source-carrier and explicit checker fixtures also run in the full
	// package; this fixture reaches the real registry with a complete carrier.
	s := Scene{World: "EURO6", GraphicsOptions: 3, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, SkyMask: make([]byte, 64000), MaterialCarrier: &Carrier{CS: make([]byte, 64000), DS: make([]byte, 64000)}}
	p := make([]byte, 64000)
	st, e := r.Draw(s, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range s.SkyMask {
		if SurfaceOwner(v).ApprovedGround() {
			t.Fatal("carrier feedback replaced by approximation")
		}
	}
	if st.SourceMemoryFallbackPrimitives != 0 {
		t.Fatal("complete carrier fallback")
	}
}
