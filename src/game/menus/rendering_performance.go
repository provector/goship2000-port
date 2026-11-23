package menus

import (
	"runtime"
	"sort"
)

type sceneFrameTiming struct {
	GPS              bool    `json:"gps_modal"`
	Target           [2]int  `json:"indexed_target"`
	Frame            int     `json:"frame"`
	Window           [2]int  `json:"window"`
	CPUCompositionMS float64 `json:"cpu_composition_ms"`
	HostFrameMS      float64 `json:"host_frame_ms"`
	Redrawn          bool    `json:"scene_redrawn"`
	Primitives       int     `json:"primitive_candidates"`
	CameraMode       int     `json:"camera_mode"`
}

func scenePerformanceReport(samples []sceneFrameTiming) map[string]any {
	summarize := func(values []float64) map[string]any {
		if len(values) == 0 {
			return map[string]any{"samples": 0}
		}
		sort.Float64s(values)
		q := func(p float64) float64 { return values[int(float64(len(values)-1)*p)] }
		over, over20, over33 := 0, 0, 0
		for _, n := range values {
			if n > 16.667 {
				over++
			}
			if n > 20 {
				over20++
			}
			if n > 33.333 {
				over33++
			}
		}
		return map[string]any{"samples": len(values), "p50_ms": q(.5), "p95_ms": q(.95), "p99_ms": q(.99), "maximum_ms": values[len(values)-1], "over_16_667_ms": over, "over_20_ms": over20, "over_33_333_ms": over33}
	}
	var cpu, host, redraw, simulation []float64
	for _, s := range samples {
		cpu = append(cpu, s.CPUCompositionMS)
		if !s.GPS {
			simulation = append(simulation, s.CPUCompositionMS)
		}
		host = append(host, s.HostFrameMS)
		if s.Redrawn {
			redraw = append(redraw, s.CPUCompositionMS)
		}
	}
	return map[string]any{"platform": runtime.GOOS + "/" + runtime.GOARCH, "go_version": runtime.Version(), "menu_logical_resolution": [2]int{320, 200}, "simulation_resolution": "per-sample indexed_target; source1=320x200, experimental2=640x400", "samples": samples, "all_composition": summarize(cpu), "simulation_only_composition": summarize(simulation), "redrawn_composition": summarize(redraw), "host_frame_including_target_fps_wait": summarize(host), "limits": "Own native window. Host frame samples include pacing, capture/readback, compositor and resizing. Composition includes CPU indexed render/HUD and palette expansion, not simulation. GPU vendor/driver is in raylib startup log. No original-frame equivalence or hard-real-time guarantee."}
}
