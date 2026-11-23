package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

type renderRunResult struct {
	State  []byte
	Timing map[string]any
}

func renderPresentationRun(t *testing.T, mode string, fps, scale int) renderRunResult {
	t.Helper()
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	p.Scene.RenderScale = scale
	p.Frame = make([]byte, p.Scene.TargetPixels())
	p.worldFrame = make([]byte, p.Scene.TargetPixels())
	p.presentation.mode = mode
	p.resetPresentation()
	type event struct {
		at       time.Duration
		held     [4]bool
		commands []int
		defenses []int
		target   bool
	}
	events := []event{{280 * time.Millisecond, [4]bool{false, false, true, false}, nil, []int{2}, false}, {760 * time.Millisecond, [4]bool{true, false, true, false}, nil, []int{0}, false}, {1110 * time.Millisecond, [4]bool{}, []int{5}, []int{1}, false}, {1480 * time.Millisecond, [4]bool{false, true, false, false}, nil, []int{2, 3}, false}, {1740 * time.Millisecond, [4]bool{}, nil, nil, true}}
	feed := func(at time.Duration, held [4]bool, commands []int) {
		if e := c.Handle(Actions{InputTime: at, FlightHeldValid: true, SceneDirections: held, FlightCommands: commands}); e != nil {
			t.Fatal(e)
		}
	}
	feed(0, [4]bool{}, nil)
	ei := 0
	var held [4]bool
	var last time.Duration
	frames, updates := []float64{}, []float64{}
	drawMutationChecks := 0
	for f := 1; f <= fps*2; f++ {
		now := time.Duration(f) * time.Second / time.Duration(fps)
		for ei < len(events) && events[ei].at <= now {
			e := events[ei]
			held = e.held
			feed(e.at, held, e.commands)
			if er := c.Handle(Actions{InputTime: e.at, DefenseCommands: e.defenses, Backspace: e.target, FlightHeldValid: true, SceneDirections: held}); er != nil {
				t.Fatal(er)
			}
			ei++
		}
		feed(now, held, nil)
		oldSteps := p.Flight.Steps
		start := time.Now()
		if e := c.AdvanceRendering(float64(now-last) / float64(time.Second)); e != nil {
			t.Fatal(e)
		}
		elapsed := float64(time.Since(start)) / 1e6
		last = now
		if p.Flight.Steps != oldSteps {
			updates = append(updates, elapsed)
		}
		if c.Screen != ScreenScene || p.Flight.Failure != "" || p.Flight.State.Terminal != nil {
			t.Fatalf("unplannedstop %s %s %+v", c.Screen, p.Flight.Failure, p.Flight.State.Terminal)
		}
		before, _ := json.Marshal(struct {
			Init, Flight, Env, Camera any
			Phase                     uint64
		}{p.Initialized, p.Flight, p.Scene.Environment, p.ViewControls, p.Visual.Phase})
		start = time.Now()
		c.Frame(false)
		frames = append(frames, float64(time.Since(start))/1e6)
		world := append([]byte(nil), p.worldFrame...)
		logicalDisplay := append([]byte(nil), p.avionicsReference...)
		p.Dirty = true
		c.Frame(false)
		after, _ := json.Marshal(struct {
			Init, Flight, Env, Camera any
			Phase                     uint64
		}{p.Initialized, p.Flight, p.Scene.Environment, p.ViewControls, p.Visual.Phase})
		if !bytes.Equal(before, after) || !bytes.Equal(world, p.worldFrame) || !bytes.Equal(logicalDisplay, p.avionicsReference) {
			t.Fatal("extra physical render mutated authoritative or retained logical state")
		}
		drawMutationChecks++
		if p.Error != nil {
			t.Fatal(p.Error)
		}
	}
	state, e := json.Marshal(struct {
		Init, Flight, Env, Camera any
		Phase                     uint64
	}{p.Initialized, p.Flight, p.Scene.Environment, p.ViewControls, p.Visual.Phase})
	if e != nil {
		t.Fatal(e)
	}
	summary := func(v []float64) map[string]any {
		sort.Float64s(v)
		if len(v) == 0 {
			return map[string]any{"count": 0}
		}
		return map[string]any{"count": len(v), "p50_ms": v[len(v)/2], "p95_ms": v[(len(v)-1)*95/100], "p99_ms": v[(len(v)-1)*99/100], "max_ms": v[len(v)-1]}
	}
	hash := sha256.Sum256(state)
	return renderRunResult{state, map[string]any{"mode": mode, "fps": fps, "scale": scale, "world": p.Scene.World, "frame_cpu": summary(frames), "advance_calls_with_logical_update": summary(updates), "logical_steps": p.Flight.Steps, "visual_steps": p.Visual.Phase, "instances": len(p.Scene.Instances), "last_raster_stats": p.Stats, "state_sha256": hex.EncodeToString(hash[:]), "extra_draw_mutation_checks": drawMutationChecks, "scope": "Actual accepted pass-through native mission,2s normalized input; complete indexed world+cockpit frame. Advance includes logical flight/contact/defense plus source reference-frame commits. No native GPU upload/readback/audio/capture. Draw duplicates for ownership tests and concurrent compiler activity may affect timing; not isolated steady60FPS certification."}}
}

func TestPresentationModesAndResolutions(t *testing.T) {
	var baseline []byte
	results := []map[string]any{}
	for _, mode := range []string{"smooth", "stepped"} {
		for _, fps := range []int{30, 60, 144} {
			for _, scale := range []int{1, 2} {
				name := fmt.Sprintf("%s-%d-%d", mode, fps, scale)
				t.Run(name, func(t *testing.T) {
					r := renderPresentationRun(t, mode, fps, scale)
					if baseline == nil {
						baseline = append([]byte(nil), r.State...)
					} else if !bytes.Equal(baseline, r.State) {
						t.Fatalf("authoritative state differs for%s", name)
					}
					results = append(results, r.Timing)
				})
			}
		}
	}
	if out := os.Getenv("GS2000_L1_FRAME_REPORT"); out != "" {
		f, e := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if e = json.NewEncoder(f).Encode(results); e != nil {
			t.Fatal(e)
		}
	}
}
