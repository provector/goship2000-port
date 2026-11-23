package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	a "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func benchmarkScenes(t testing.TB) (*a.OfficeAssets, map[string]render3d.Scene) {
	t.Helper()
	root, file := os.Getenv("GS2000_TEST_MISSIONS"), os.Getenv("GS2000_R81_SCENES")
	if file == "" {
		file = "../../../test-data/render-scenes/scenes.json"
	}
	if root == "" {
		t.Skip("explicit prepared assets required")
	}
	assets, e := a.LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var scenes map[string]render3d.Scene
	if e = json.Unmarshal(raw, &scenes); e != nil {
		t.Fatal(e)
	}
	return assets, scenes
}

// Same real draw/composition path, finite motion and option matrix. The JSON
// scenes contain only supplied world/camera/instance data, no personal state.
func TestRenderMatrix(t *testing.T) {
	out := os.Getenv("GS2000_R81_CAPTURE")
	if out == "" {
		t.Skip("explicit renderer capture")
	}
	assets, scenes := benchmarkScenes(t)
	r, e := render3d.New(assets.World)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(out, 0700); e != nil {
		t.Fatal(e)
	}
	var reports []map[string]any
	for _, name := range []string{"euro6-pad", "euro6-brown", "euro7-pad", "euro8-field", "euro-trees", "jungle", "gulf", "antarctic"} {
		for _, scale := range []int{1, 2} {
			for motion := 0; motion < 17; motion++ {
				s := scenes[name]
				s.RenderScale = scale
				switch motion {
				case 1:
					s.Camera.H0 += 32
				case 2:
					s.Camera.H0 += 64
				case 3:
					s.Camera.Angles[1] += 256
				case 4:
					s.Camera.Angles[2] += 256
				case 5:
					s.Camera.Height += 256
				case 6:
					s.Camera.Height += 1024
				default:
					if motion >= 7 {
						s.Camera.H0 += uint32((motion - 6) * 8)
						s.Camera.Angles[1] += uint16((motion - 6) * 32)
					}
				}
				if strings.HasPrefix(s.World, "EURO") {
					s.SkyMask = make([]byte, s.TargetPixels())
				}
				pixels := make([]byte, s.TargetPixels())
				pal, e := r.Palette(s, nil)
				if e != nil {
					t.Fatal(e)
				}
				p := &RenderingPreview{Scene: s, Renderer: r, Frame: pixels, Palette: pal, skyMask: s.SkyMask}
				c := &FrontEnd{Screen: ScreenScene, Office: &Office{Assets: assets}, Rendering: p}
				before, _ := json.Marshal(s)
				st, e := r.Draw(s, nil, pixels)
				if e != nil {
					t.Fatal(e)
				}
				im, e := c.RenderRGBA(false)
				if e != nil {
					t.Fatal(e)
				}
				first := bytes.Clone(im.Pix)
				for n := 0; n < 3; n++ {
					if _, e = r.Draw(s, nil, pixels); e != nil {
						t.Fatal(e)
					}
					im, e = c.RenderRGBA(false)
					if e != nil || !bytes.Equal(first, im.Pix) {
						t.Fatal(name, "unstable identical redraw", e)
					}
				}
				after, _ := json.Marshal(s)
				if !bytes.Equal(before, after) {
					t.Fatal("render mutated scene")
				}
				label := fmt.Sprintf("%s-s%d-m%d", name, scale, motion)
				f, e := os.Create(filepath.Join(out, label+".png"))
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(f, im)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
				reports = append(reports, map[string]any{"name": label, "scene": s, "stats": st, "indexed_sha256": fmt.Sprintf("%x", sha256.Sum256(pixels)), "rgba_sha256": fmt.Sprintf("%x", sha256.Sum256(first))})
			}
		}
	}
	raw, _ := json.MarshalIndent(reports, "", "  ")
	if e = os.WriteFile(filepath.Join(out, "frames.json"), raw, 0600); e != nil {
		t.Fatal(e)
	}
}

func benchmarkLatency(t testing.TB, name string, draw func(), samples int) map[string]any {
	t.Helper()
	for i := 0; i < 90; i++ {
		draw()
	}
	runtime.GC()
	var a, z runtime.MemStats
	runtime.ReadMemStats(&a)
	values := make([]float64, samples)
	start := time.Now()
	for i := range values {
		now := time.Now()
		draw()
		values[i] = float64(time.Since(now)) / 1e6
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&z)
	sort.Float64s(values)
	return map[string]any{"name": name, "frames": samples, "seconds": elapsed.Seconds(), "median_ms": values[samples/2], "p95_ms": values[(samples-1)*95/100], "p99_ms": values[(samples-1)*99/100], "max_ms": values[samples-1], "allocations_per_frame": float64(z.Mallocs-a.Mallocs) / float64(samples), "bytes_per_frame": float64(z.TotalAlloc-a.TotalAlloc) / float64(samples), "gc_cycles": z.NumGC - a.NumGC, "gc_pause_ms": float64(z.PauseTotalNs-a.PauseTotalNs) / 1e6}
}

func TestLatencyMatrix(t *testing.T) {
	out := os.Getenv("GS2000_R81_LATENCY")
	if out == "" {
		t.Skip("explicit latency probe")
	}
	assets, scenes := benchmarkScenes(t)
	r, e := render3d.New(assets.World)
	if e != nil {
		t.Fatal(e)
	}
	var results []map[string]any
	for _, name := range []string{"euro6-pad", "euro6-brown", "euro7-pad", "euro8-field", "euro-trees", "jungle", "gulf", "antarctic"} {
		for _, scale := range []int{1, 2} {
			s := scenes[name]
			s.RenderScale = scale
			if strings.HasPrefix(s.World, "EURO") {
				s.SkyMask = make([]byte, s.TargetPixels())
			}
			pal, e := r.Palette(s, nil)
			if e != nil {
				t.Fatal(e)
			}
			pixels := make([]byte, s.TargetPixels())
			p := &RenderingPreview{Scene: s, Renderer: r, Frame: pixels, Palette: pal, skyMask: s.SkyMask}
			c := &FrontEnd{Screen: ScreenScene, Office: &Office{Assets: assets}, Rendering: p}
			results = append(results, benchmarkLatency(t, fmt.Sprintf("%s-s%d", name, scale), func() {
				if _, e := r.Draw(s, nil, pixels); e != nil {
					t.Fatal(e)
				}
				if _, e := c.RenderRGBA(false); e != nil {
					t.Fatal(e)
				}
			}, 600))
		}
	}
	raw, _ := json.MarshalIndent(results, "", "  ")
	if e = os.WriteFile(out, raw, 0600); e != nil {
		t.Fatal(e)
	}
}

// An existing complete R6 recording supplies the ordinary actor population,
// cockpit and normal MFD. Resume is practice-only with no writable career.
func TestCockpitLatency(t *testing.T) {
	out := os.Getenv("GS2000_R81_COCKPIT")
	if out == "" {
		t.Skip("explicit ordinary cockpit probe")
	}
	assets, _ := benchmarkScenes(t)
	raw, e := os.ReadFile(os.Getenv("GS2000_R7_FILM"))
	if e != nil {
		t.Fatal(e)
	}
	f, e := replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	var results []map[string]any
	for _, scale := range []int{1, 2} {
		if os.Getenv("GS2000_R81_ONLY_SCALE") == "2" && scale != 2 {
			continue
		}
		cfg := configuration.Defaults()
		cfg.SimulationRenderScale = scale
		q, e := NewFilmPlayback(assets, f, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for q.reachedSteps < 16 && !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
		if e = q.Resume(); e != nil {
			t.Fatal(e)
		}
		c := q.Engine
		p := q.Preview()
		p.Flight.Paused = true
		p.presentation.mode = "smooth"
		t.Log("ordinary", p.Scene.World, "instances", len(p.Scene.Instances), "scale", scale, "camera", p.Cockpit.CameraMode, "MFD", p.Cockpit.PrimaryMFD)
		result := benchmarkLatency(t, fmt.Sprintf("ordinary-cockpit-s%d", scale), func() {
			p.Dirty = true
			if _, e = c.RenderRGBA(false); e != nil {
				t.Fatal(e)
			}
		}, 1200)
		result["world"], result["instances"] = p.Scene.World, len(p.Scene.Instances)
		results = append(results, result)
	}
	raw, _ = json.MarshalIndent(results, "", "  ")
	if e = os.WriteFile(out, raw, 0600); e != nil {
		t.Fatal(e)
	}
}

// Real world raster plus normal Frame/RenderRGBA composition; setup and assets
// excluded. No intentional wait, simulation advance,PNG encoding or GPU upload.
func BenchmarkScenes(b *testing.B) {
	assets, scenes := benchmarkScenes(b)
	r, err := render3d.New(assets.World)
	if err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"euro6-pad", "euro6-brown", "euro7-pad", "euro8-field", "euro-trees", "jungle", "gulf", "antarctic"} {
		for _, scale := range []int{1, 2} {
			b.Run(fmt.Sprintf("%s/scale%d", name, scale), func(b *testing.B) {
				s := scenes[name]
				s.RenderScale = scale
				if strings.HasPrefix(s.World, "EURO") {
					s.SkyMask = make([]byte, s.TargetPixels())
				}
				pixels := make([]byte, s.TargetPixels())
				palette, e := r.Palette(s, nil)
				if e != nil {
					b.Fatal(e)
				}
				p := &RenderingPreview{Scene: s, Renderer: r, Frame: pixels, Palette: palette, skyMask: s.SkyMask}
				c := &FrontEnd{Screen: ScreenScene, Office: &Office{Assets: assets}, Rendering: p}
				b.ReportAllocs()
				b.ResetTimer()
				for n := 0; n < b.N; n++ {
					stats, e := r.Draw(s, nil, pixels)
					if e != nil {
						b.Fatal(e)
					}
					p.Stats = stats
					if _, e = c.RenderRGBA(false); e != nil {
						b.Fatal(e)
					}
				}
			})
		}
	}
}
