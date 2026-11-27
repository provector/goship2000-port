package render3d

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func padScene(t *testing.T, r *Renderer) Scene {
	t.Helper()
	root := os.Getenv("GS2000_Q32_GROUND_SCENES")
	if root == "" {
		root = testfixture.Path("ground-scenes")
	}
	raw, e := os.ReadFile(filepath.Join(root, "pad-scene.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s Scene
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	x, z := int(s.Camera.H0>>13), int(s.Camera.H1>>13)
	best, dist := -1, int(^uint(0)>>1)
	for at, id := range r.Assets.Worlds["EURO6"].Grid {
		if id != 22 {
			continue
		}
		xx, zz := at%64, 63-at/64
		d := (xx-x)*(xx-x) + (zz-z)*(zz-z)
		if d < dist {
			best, dist = at, d
		}
	}
	if best < 0 {
		t.Fatal("actual cell22 unavailable")
	}
	dx, dz := int32((best%64-x)*8192), int32((63-best/64-z)*8192)
	s.World = "EURO6"
	s.Camera.H0 = uint32(int32(s.Camera.H0) + dx)
	s.Camera.H1 = uint32(int32(s.Camera.H1) + dz)
	for i := range s.Instances {
		s.Instances[i].H0 = uint32(int32(s.Instances[i].H0) + dx)
		s.Instances[i].H1 = uint32(int32(s.Instances[i].H1) + dz)
	}
	return s
}
func TestB4PadWinningWritesAndOptions(t *testing.T) {
	r, e := New(materialsBundle(t))
	if e != nil {
		t.Fatal(e)
	}
	source := padScene(t, r)
	for _, scale := range []int{1, 2} {
		for _, palette := range []string{"DAY", "GREY", "DARK"} {
			for _, options := range []byte{0, 1, 2, 3} {
				s := source
				s.RenderScale = scale
				s.PaletteBank = palette
				s.GraphicsOptions = options
				s.GroundOwners = make([]byte, s.TargetPixels())
				s.SkyMask = make([]byte, s.TargetPixels())
				pixels := make([]byte, s.TargetPixels())
				st, e := r.Draw(s, nil, pixels)
				if e != nil {
					t.Fatal(e)
				}
				written, holes := 0, 0
				for at, v := range s.GroundOwners {
					base, wrote, pad := SurfaceOwner(v).Pad()
					if pad && base == SurfaceBrown {
						if wrote {
							written++
							if pixels[at] != 0xec {
								t.Fatal("owner differs from actual EC write")
							}
						} else {
							holes++
							if pixels[at]&0xfc != 0xb4 {
								t.Fatal("underlying source family lost through hole")
							}
						}
					}
					if pixels[at] == 0x0e && pad {
						t.Fatal("marking retained pad layer")
					}
				}
				if options&1 != 0 {
					want := 102
					if scale == 2 {
						want = 472
					}
					if written != want || holes == 0 {
						t.Fatal(scale, palette, options, written, holes, want, st)
					}
				} else if written+holes != 0 {
					t.Fatal("Fancy-off guessed a ground treatment")
				}
				// Reused outputs must not retain any owner from a previous frame.
				canonical := bytes.Clone(pixels)
				owners := bytes.Clone(s.GroundOwners)
				for i := range s.GroundOwners {
					s.GroundOwners[i] = 0xff
					s.SkyMask[i] = 0xff
				}
				if _, e = r.Draw(s, nil, pixels); e != nil {
					t.Fatal(e)
				}
				if !bytes.Equal(canonical, pixels) || !bytes.Equal(owners, s.GroundOwners) {
					t.Fatal("stale ownership across frames")
				}
				if _, e = r.Palette(s, nil); e != nil {
					t.Fatal(e)
				}
			}
		}
	}
}

func TestSameInkAndUnapprovedPadBackground(t *testing.T) {
	b := materialsBundle(t)
	r, e := New(b)
	if e != nil {
		t.Fatal(e)
	}
	source := padScene(t, r)
	// An unrelated model deliberately shares the exact pad primitive pointers in
	// this supplied renderer fixture. Source/model use must still separate them.
	clone := *b.Geometries["model/131"]
	clone.ID = "model/034"
	b.Geometries[clone.ID] = &clone
	for _, model := range []int{130, 34} {
		s := source
		s.Instances = append([]Instance(nil), source.Instances...)
		for i := range s.Instances {
			if s.Instances[i].ModelID == 131 {
				s.Instances[i].ModelID = model
			}
		}
		s.SkyMask = make([]byte, s.TargetPixels())
		s.GroundOwners = make([]byte, s.TargetPixels())
		p := make([]byte, s.TargetPixels())
		if _, e = r.Draw(s, nil, p); e != nil {
			t.Fatal(e)
		}
		for _, v := range s.GroundOwners {
			if _, _, ok := SurfaceOwner(v).Pad(); ok {
				t.Fatal("same-ink model acquired pad role", model)
			}
		}
	}
	// The real pad on an unclassified background keeps actual screen-door holes.
	for _, scale := range []int{1, 2} {
		s := Scene{World: "EURO7", RenderScale: scale, GraphicsOptions: 3, View: FullView(), SkyMask: make([]byte, 64000*scale*scale)}
		p := bytes.Repeat([]byte{0x19}, len(s.SkyMask))
		if _, e = r.DrawModelPreview(s, nil, p, ModelPreview{ModelID: 131, Angles: [3]uint16{0x800, 0, 0}, Translation: world.Vertex{0, 0, 2048}}); e != nil {
			t.Fatal(e)
		}
		for _, v := range s.SkyMask {
			if SurfaceOwner(v).ApprovedGround() {
				t.Fatal("unapproved background guessed")
			}
		}
	}
}
