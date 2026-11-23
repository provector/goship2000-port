package render3d

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestScaledProjectionRetainsFraction(t *testing.T) {
	f := frame{scene: Scene{RenderScale: 2}, view: Scene{RenderScale: 2}.rasterView()}
	// Native x=1,z=6: source256/6 trunc42, scaled512/6 trunc85, not84.
	p, e := f.project(Vector{1 << 16, 0, 6 << 16}, true)
	if e != nil || p != (Point{405, 140}) {
		t.Fatalf("scaled rational projection %+v %v", p, e)
	}
	if f.view.Bounds != (Rect{0, 0, 639, 399}) {
		t.Fatal(f.view)
	}
}
func TestHighResolutionDestinationsAndPatternCoordinates(t *testing.T) {
	dst := bytes.Repeat([]byte{0x62}, 256000)
	dst[641] = 0x10
	if _, e := FillPolygon(dst, []Point{{0, 0}, {3, 0}, {3, 3}, {0, 3}}, 0, Rect{0, 0, 639, 399}, 0, nil); e != nil {
		t.Fatal(e)
	}
	if dst[640] != 0x61 || dst[641] != 0x10 {
		t.Fatal("destination remap used a sampled low-resolution target", dst[640:642])
	}
	clear(dst)
	fillSpan(dst, 4, 0, 9, 0x8979, 0, nil)
	if !bytes.Equal(dst[4*640:4*640+10], []byte{0x79, 0x79, 0x89, 0x89, 0x79, 0x79, 0x89, 0x89, 0x79, 0x79}) {
		t.Fatal("source checker lost logical apparent size")
	}
}
func TestGeometryResolutionAndDrawingPurity(t *testing.T) {
	r, _ := New(tinyBundle())
	sc := Scene{World: "GULF5", Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}, Angles: [3]uint16{}}, View: FullView()}
	sc.Instances = []Instance{{ID: "fractional-edge", ModelID: 0, Pose: Pose{H0: 200017, H1: 200301, Height: 515}}}
	state := &VisualState{}
	before, _ := json.Marshal(state)
	low := make([]byte, 64000)
	if _, e := r.Draw(sc, state, low); e != nil {
		t.Fatal(e)
	}
	sc.RenderScale = 2
	hi := make([]byte, 256000)
	if _, e := r.Draw(sc, state, hi); e != nil {
		t.Fatal(e)
	}
	detailed := false
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			at := y*2*640 + x*2
			if hi[at] != hi[at+1] || hi[at] != hi[at+640] || hi[at] != hi[at+641] {
				detailed = true
			}
		}
	}
	if !detailed {
		t.Fatal("high-resolution geometry was only a nearest2x completed image")
	}
	after, _ := json.Marshal(state)
	if !bytes.Equal(before, after) {
		t.Fatal("draw advanced logical state")
	}
}

func TestSourceFramePreservationAndPerformance(t *testing.T) {
	out := os.Getenv("GS2000_L_RENDER_REPORT")
	if out == "" {
		t.Skip("explicit disposable research report path")
	}
	rows := []map[string]any{}
	for _, name := range []string{"GULF5", "PHIL1"} {
		r, e := New(originalBundle(t, name))
		if e != nil {
			t.Fatal(e)
		}
		sc := sourceTrainingScene(t)
		sc.World = name
		if name == "PHIL1" {
			for i, v := range r.Assets.Worlds[name].Grid {
				if v == 23 && i%64 > 4 && i%64 < 59 && i/64 > 4 && i/64 < 59 {
					sc.Camera = Camera{Pose: Pose{H0: uint32(i%64*8192 + 4096), H1: uint32((63-i/64)*8192 + 4096), Height: 800}, Angles: [3]uint16{0xf400, 0x1000, 0}}
					break
				}
			}
			for i := range sc.Instances {
				sc.Instances[i].H0 = sc.Camera.H0 + uint32(i*128)
				sc.Instances[i].H1 = sc.Camera.H1 + 1024
			}
		}
		for _, scale := range []int{1, 2} {
			sc.RenderScale = scale
			dst := make([]byte, sc.TargetPixels())
			times := []float64{}
			var st Stats
			for j := 0; j < 120; j++ {
				start := time.Now()
				st, e = r.Draw(sc, &VisualState{}, dst)
				if e != nil {
					t.Fatal(e)
				}
				times = append(times, float64(time.Since(start))/1e6)
			}
			sort.Float64s(times)
			rows = append(rows, map[string]any{"world": name, "scale": scale, "p50_ms": times[60], "p95_ms": times[114], "p99_ms": times[118], "max_ms": times[119], "frames": 120, "stats": st, "scope": "complete populated pure world raster, no window, cockpit, readback/capture, authoritative update or audio; PHIL supplied placements"})
		}
	}
	raw, _ := json.MarshalIndent(rows, "", "  ")
	if e := os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(out, raw, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestSourceStyleMatchesRetainedK1Comparison(t *testing.T) {
	// Retained pre-L source-style comparison is a regression witness, not proof
	// of original rendering. Hand/source-operand checks above remain independent.
	root := filepath.Join(testfixture.Path("material-sequence"))
	if _, e := os.Stat(root); os.IsNotExist(e) {
		t.Skip("optional retained Step34 source-style comparison")
	}
	r, e := New(originalBundle(t, "PHIL1"))
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"00", "01", "12", "23"} {
		raw, e := os.ReadFile(filepath.Join(root, "source-"+name+".json"))
		if e != nil {
			t.Fatal(e)
		}
		var v struct{ Scene Scene }
		if e = json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		want, e := os.ReadFile(filepath.Join(root, "source-"+name+".indices"))
		if e != nil {
			t.Fatal(e)
		}
		got := make([]byte, 64000)
		if _, e = r.Draw(v.Scene, &VisualState{}, got); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(got, want) {
			n := 0
			literal := 0
			// These words come from recognized source geometry, not this frame's
			// output. VGA0515..051d emits DL for an odd single-pixel span and
			// never reads CS/DS. The old diagnostic hatch incorrectly replaced it.
			seedWords := map[byte]bool{}
			var seeds func(world.Group)
			seeds = func(g world.Group) {
				if p := g.Primitive; p != nil && p.Colour != 0 && p.Colour < 256 {
					seedWords[byte(p.Colour)] = true
				}
				for _, child := range g.Children {
					seeds(child)
				}
			}
			for _, g := range r.Assets.Geometries {
				for _, group := range g.PlaneGroups {
					seeds(group)
				}
				for _, group := range g.DirectGroups {
					seeds(group)
				}
			}
			for i := range got {
				if got[i] != want[i] {
					// L.3 corrected the independently checked VGA boundary-only
					// span rejection. Keep this historical capture unchanged and
					// compare its interior; boundary semantics have a hand-case test.
					x := i % Width
					b := v.Scene.view().Bounds
					if x == b.Left || x == b.Right {
						continue
					}
					if v.Scene.GraphicsOptions&1 != 0 && seedWords[got[i]] && want[i] == got[i]&0xfc|byte(((x>>2)^((i/Width)>>2))&3) {
						literal++
						continue
					}
					n++
				}
			}
			if n != 0 {
				t.Fatalf("source frame%s differs at%d interior indices", name, n)
			}
			t.Logf("historical frame%s: %d independently justified plain-DL corrections; historical pixels retained", name, literal)
		}
	}
}

func TestProjectionAndClippedCoverage(t *testing.T) {
	for _, tc := range []struct {
		name            string
		w, h, body, top int
	}{{"800x600", 800, 600, 600, 0}, {"1024x768", 1024, 768, 768, 0}, {"1280x1024", 1280, 1024, 960, 32}} {
		s := Scene{Resolution: tc.name}
		f := frame{scene: s, view: s.rasterView()}
		if w, h := s.Dimensions(); w != tc.w || h != tc.h || f.view.Bounds != (Rect{0, tc.top, w - 1, tc.top + tc.body - 1}) {
			t.Fatal(tc, f.view)
		}
		p, err := f.project(Vector{1 << 16, 1 << 16, 6 << 16}, true)
		want := Point{tc.w/2 + 256*tc.w/(6*320), tc.top + 70*tc.body/200 - 256*tc.body/(6*200)}
		if err != nil || p != want {
			t.Fatal(tc, p, want, err)
		}
		dst := make([]byte, tc.w*tc.h)
		b := f.view.Bounds
		if _, err := FillPolygon(dst, []Point{{-180000, -40000}, {300000, -45000}, {320000, 75000}, {-220000, 78000}}, 0x898c, b, 0, nil); err != nil {
			t.Fatal(err)
		}
		for y := 0; y < tc.h; y++ {
			for x := 0; x < tc.w; x++ {
				want := byte(0)
				if y >= tc.top && y < tc.top+tc.body {
					want = 0x8c
					if (x*320/tc.w+(y-tc.top)*200/tc.body)&1 != 0 {
						want = 0x89
					}
				}
				if dst[y*tc.w+x] != want {
					t.Fatalf("%s at %d,%d", tc.name, x, y)
				}
			}
		}
		clipped, err := clipDepthPolygon([]Vector{{-16 << 16, 0, 2 << 16}, {10 << 16, 16 << 16, 32 << 16}, {12 << 16, -16 << 16, 32 << 16}}, 8, 8192)
		if err != nil || len(clipped) != 4 {
			t.Fatal("near crossing", err)
		}
		for _, v := range clipped {
			if v[2] < 8<<16 {
				t.Fatal("unclipped depth")
			}
			if _, err := f.project(v, true); err != nil {
				t.Fatal(err)
			}
		}
	}
}
