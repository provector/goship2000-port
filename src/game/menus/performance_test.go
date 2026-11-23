package menus

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

// Optional bounded measurement: complete populated logical update + CPU frame/
// palette composition. No capture/PNG/GPU/audio transport is charged as rendering.
func TestPopulatedFrameCost(t *testing.T) {
	out := os.Getenv("GS2000_N6_PERFORMANCE")
	if out == "" {
		t.Skip("explicit new performance output required")
	}
	var rows []map[string]any
	for _, scale := range []int{1, 2} {
		for _, request := range []int16{0, 4, 16} {
			c := developmentScenarioController(t, "n6-compression")
			p, r := c.Rendering, c.Rendering.Flight
			p.Scene.RenderScale = scale
			p.Frame = make([]byte, p.Scene.TargetPixels())
			p.worldFrame = make([]byte, p.Scene.TargetPixels())
			p.resetPresentation()
			if e := c.Handle(Actions{Cancel: true}); e != nil {
				t.Fatal(e)
			}
			c.Handle(Actions{})
			r.Compression.Request = request
			var samples []float64
			startSteps := r.Steps
			for i := 0; i < 200; i++ {
				start := time.Now()
				if e := c.AdvanceRendering(1.0 / 60); e != nil {
					t.Fatal(e)
				}
				if _, e := c.RenderRGBA(false); e != nil {
					t.Fatal(e)
				}
				c.MarkScenePresented()
				p.takeSceneAudio()
				samples = append(samples, float64(time.Since(start).Nanoseconds())/1e6)
			}
			sort.Float64s(samples)
			rate := int(request)
			if rate == 0 {
				rate = 1
			}
			rows = append(rows, map[string]any{"resolution": fmt.Sprintf("%dx%d", 320*scale, 200*scale), "nominal_rate": rate, "actual_final_request": r.Compression.Request, "logical_updates": r.Steps - startSteps, "host_frames": len(samples), "median_ms": samples[100], "p95_ms": samples[189], "p99_ms": samples[197], "max_ms": samples[199], "primitive_candidates": p.Stats.Primitives})
		}
	}
	raw, e := json.MarshalIndent(map[string]any{"scope": "Real generated Gulf2 Apache hover, complete logical updates plus CPU indexed/HUD/palette composition,200 host frames at60Hz; preparation excluded; no reduced scene content. GPU/upload/compositor/audio worker/capture excluded. Not a universal60FPS or ARM result.", "rows": rows}, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = f.Write(raw); e != nil {
		t.Fatal(e)
	}
}
