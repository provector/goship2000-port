package menus

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

// An ordinary source-generated enemy ground formation: supplied Captain history,
// Single/EURO seed8, unchanged OBJECT/PATH/actor poses and source speeds. The
// player's normal rotor command prevents the base-end prompt during observation.
func movingTimingMovingController(t *testing.T, scale, fps int, mode string) (*FrontEnd, int) {
	t.Helper()
	c := authoredUIController(t)
	s := c.Store.State()
	s.SelectedPilot().MissionType = gamestate.MissionSingle
	if err := c.Store.Commit(s); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	seed := int64(8)
	c.MissionSeed = &seed
	c.Runtime = c.Runtime.Effective()
	c.Runtime.SimulationRenderScale = scale
	c.Runtime.PresentationFPS = fps
	c.Runtime.PresentationMode = mode
	readyMission(t, c)
	startRendering(t, c)
	jFinishFade(t, c)
	c.AbandonFilmRecording()
	if c.Mission.Data.World != "EURO7.WRL" || c.Mission.Data.ObjectHash != "ae3d968bc878d17906d6f5715f1b98638f64ed9e64559cbe80ac7946028992d7" {
		t.Fatal("hash-bound ordinary generated fixture changed", c.Mission.Data.World, c.Mission.Data.ObjectHash)
	}
	header := -1
	for i, e := range c.Rendering.Initialized.Entities {
		if e.Kind == 0x40 {
			header = i
			break
		}
	}
	if header != 39 || c.Rendering.Initialized.Entities[header].RawMotion != 20 || c.Rendering.Initialized.Entities[header+1].LeaderRow != header {
		t.Fatal("actual group/header/speed changed", header)
	}
	c.Rendering.applyFlightCommand(2, c.Office.Assets.Cockpit)
	if c.Rendering.Flight.State.Rotor == 0 {
		t.Fatal("ordinary rotor command failed")
	}
	return c, header
}

type movingSample struct {
	ChildH0, ChildH1 uint32
	Tick             uint64
	H0, H1           uint32
	Heading          uint16
	Cursor           uint16
	Participation    uint8
	Hash             string
}

func movingSampleAt(c *FrontEnd, h int) movingSample {
	p := c.Rendering
	e := p.Initialized.Entities[h]
	raw := filmJSON(struct {
		Entities, Route any
		RNG             uint16
	}{p.Initialized.Entities, p.Flight.Combat.FilmState().Route, p.Initialized.RandomState})
	child := p.Initialized.Entities[h+1]
	return movingSample{Tick: p.Flight.Steps, H0: e.Pose.H0, H1: e.Pose.H1, Heading: e.Pose.Angles[1], Cursor: p.Flight.Combat.FilmState().Route.Cursor[h], Participation: e.Participation, Hash: fmt.Sprintf("%x", sha256.Sum256(raw)), ChildH0: child.Pose.H0, ChildH1: child.Pose.H1}
}
func writeTiming(t *testing.T, name string, v any) {
	t.Helper()
	if root := os.Getenv("GS2000_Q32_TIMING_OUTPUT"); root != "" {
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, name), append(b, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
func advanceMoving(t *testing.T, c *FrontEnd, elapsed time.Duration) {
	t.Helper()
	if err := c.AdvanceRendering(elapsed.Seconds()); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenScene || c.Rendering.Flight.Failure != "" {
		t.Fatal("unexpected sortie boundary", c.Screen, c.Rendering.Flight.Failure)
	}
}

func TestGeneratedMovingFixedTickAndPresentationClock(t *testing.T) {
	c, h := movingTimingMovingController(t, 1, 10, "stepped")
	trajectory := []movingSample{movingSampleAt(c, h)}
	arrival := -1
	// Keep the first ordinary route leg through its first destination arrival.
	for i := 0; i < 1200; i++ {
		advanceMoving(t, c, flight.StepDuration()+1)
		s := movingSampleAt(c, h)
		if s.Tick != uint64(len(trajectory)) {
			t.Fatal("single source quantum", s.Tick, len(trajectory))
		}
		trajectory = append(trajectory, s)
		if s.Tick <= 100 && (s.H0 != 229376+20*uint32(s.Tick) || s.H1 != 45056) {
			t.Fatal("original direct20 per source visit disagrees", s)
		}
		if s.Cursor >= 2 {
			arrival = i + 1
			break
		}
	}
	if arrival < 0 {
		t.Fatal("ordinary generated PATH did not reach first destination within bounded1200 visits", trajectory[len(trajectory)-1])
	}
	t.Logf("seed8 EURO7 header%d child%d raw20, first destination arrival tick%d pose%+v", h, h+1, arrival, trajectory[len(trajectory)-1])
	writeTiming(t, "generated-trajectory.json", trajectory)
	cases := []struct {
		name                    string
		scale, fps, compression int
		mode                    string
		cadence                 []time.Duration
	}{
		{"10fps", 1, 10, 1, "stepped", []time.Duration{100 * time.Millisecond}},
		{"60fps", 2, 60, 1, "smooth", []time.Duration{time.Second / 60}},
		{"irregular", 2, 60, 1, "smooth", []time.Duration{9 * time.Millisecond, 37 * time.Millisecond, 180 * time.Millisecond, 23 * time.Millisecond, 211 * time.Millisecond}},
		{"low5fps", 1, 5, 1, "stepped", []time.Duration{200 * time.Millisecond}},
		{"compression2-equal-simulated", 1, 10, 2, "stepped", []time.Duration{50 * time.Millisecond}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, header := movingTimingMovingController(t, tc.scale, tc.fps, tc.mode)
			if tc.compression == 2 {
				q.Rendering.Flight.Compression.Increase()
			}
			samples := []movingSample{}
			var elapsed time.Duration
			uiClock := tickAccumulator{Rate: 70}
			crossedAt := uint64(0)
			total := time.Duration((int64(arrival)*flight.SourcePITDivisor*flight.NativeClocksPerUpdate*int64(time.Second) + flight.SourcePITHz*int64(tc.compression) - 1) / (flight.SourcePITHz * int64(tc.compression)))
			for frame := 0; frame < 100000 && elapsed < total; frame++ {
				dt := min(tc.cadence[frame%len(tc.cadence)], total-elapsed)
				for n := uiClock.Advance(dt.Seconds(), false); n > 0; n-- {
					if err := q.Tick(); err != nil {
						t.Fatal(err)
					}
				}
				advanceMoving(t, q, dt)
				elapsed += dt
				s := movingSampleAt(q, header)
				if s.Tick >= uint64(len(trajectory)) {
					if s.Tick != uint64(arrival) {
						t.Fatal("frame passed final expected tick", s.Tick, arrival)
					}
					break
				}
				want := trajectory[s.Tick]
				if s != want {
					t.Fatalf("cadence changed actual trajectory at fixed simulation tick: got%+v want%+v", s, want)
				}
				if frame%100 == 0 || s.Cursor >= 2 {
					samples = append(samples, s)
				}
				if s.Cursor >= 2 {
					crossedAt = s.Tick
				}
			}
			if crossedAt != uint64(arrival) {
				t.Fatal("arrival tick", crossedAt, arrival)
			}
			// Authoritative poses/RNG cannot change from extra rendering/interpolation.
			before := movingSampleAt(q, header)
			q.Rendering.Dirty = true
			q.Frame(false)
			q.Rendering.Dirty = true
			q.Frame(false)
			if after := movingSampleAt(q, header); after != before {
				t.Fatal("draw repeated group motion", before, after)
			}
			writeTiming(t, tc.name+"-trajectory-samples.json", map[string]any{"cadence": tc.cadence, "host_credit_ns": elapsed, "compression": tc.compression, "arrival_tick": crossedAt, "samples": samples})
			t.Logf("%s final source tick%d credited host%v compression%d", tc.name, crossedAt, elapsed, tc.compression)
		})
	}
}

// Explicit opt-in real-monotonic pacing. This calls the native game scheduler,
// actor phase and raster frames without a window/OS input. A synthetic cadence
// test above must not be described as a measured wall-time observation.
func TestMovingRealMonotonicPacing(t *testing.T) {
	if os.Getenv("GS2000_Q32_TIMING_PACED") != "1" {
		t.Skip("explicit bounded real-monotonic profiling run")
	}
	type row struct {
		WallSeconds, DeltaSeconds float64
		Tick                      uint64
		H0, H1                    uint32
		Cursor                    uint16
	}
	cases := []struct {
		name       string
		scale, fps int
		mode       string
		cadence    []time.Duration
		draw       bool
	}{
		{"real10fps", 1, 10, "stepped", []time.Duration{100 * time.Millisecond}, true},
		{"real60fps", 2, 60, "smooth", []time.Duration{time.Second / 60}, true},
		{"real-irregular", 1, 10, "stepped", []time.Duration{9 * time.Millisecond, 37 * time.Millisecond, 180 * time.Millisecond, 23 * time.Millisecond, 211 * time.Millisecond}, true},
		{"real-headless-poll", 1, 60, "stepped", []time.Duration{time.Second / 60}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, h := movingTimingMovingController(t, tc.scale, tc.fps, tc.mode)
			rows := []row{}
			start := time.Now()
			previous := start
			deadline := start.Add(3 * time.Second)
			target := start
			clamps := 0
			uiClock := tickAccumulator{Rate: 70}
			for frame := 0; ; frame++ {
				target = target.Add(tc.cadence[frame%len(tc.cadence)])
				if target.After(deadline) {
					target = deadline
				}
				if delay := time.Until(target); delay > 0 {
					time.Sleep(delay)
				}
				now := time.Now()
				dt := now.Sub(previous)
				previous = now
				if dt > 250*time.Millisecond {
					clamps++
				}
				for n := uiClock.Advance(dt.Seconds(), false); n > 0; n-- {
					if err := c.Tick(); err != nil {
						t.Fatal(err)
					}
				}
				advanceMoving(t, c, dt)
				s := movingSampleAt(c, h)
				if tc.draw {
					c.Frame(false)
				}
				rows = append(rows, row{now.Sub(start).Seconds(), dt.Seconds(), s.Tick, s.H0, s.H1, s.Cursor})
				if !now.Before(deadline) {
					break
				}
			}
			elapsed := previous.Sub(start)
			s := movingSampleAt(c, h)
			want := uint64(elapsed.Nanoseconds() * flight.SourcePITHz / (flight.SourcePITDivisor * flight.NativeClocksPerUpdate * int64(time.Second)))
			if clamps == 0 && s.Tick != want {
				t.Fatal("actual host elapsed -> wrong accepted ticks", s.Tick, want, elapsed)
			}
			writeTiming(t, tc.name+".json", map[string]any{"scope": "Actual monotonic host time, native scheduler/actor/raster calls; no OS window or original DOS execution", "elapsed_seconds": elapsed.Seconds(), "ticks": s.Tick, "ticks_per_real_second": float64(s.Tick) / elapsed.Seconds(), "clamped_host_frames": clamps, "final": s, "rows": rows})
			t.Logf("%s wall%.6fs ticks%d effective%.6f/s stall-clamps%d", tc.name, elapsed.Seconds(), s.Tick, float64(s.Tick)/elapsed.Seconds(), clamps)
		})
	}
}

func TestGeneratedMovingPathMetadata(t *testing.T) {
	c, h := movingTimingMovingController(t, 1, 10, "stepped")
	t.Logf("PATH%s header%+v", filmJSON(c.Mission.Data.Paths), c.Rendering.Initialized.Entities[h])
	writeTiming(t, "fixture.json", map[string]any{"seed": 5, "world": c.Mission.Data.World, "scenario": c.Mission.Data.Scenario, "object_hash": c.Mission.Data.ObjectHash, "paths": c.Mission.Data.Paths, "header": c.Rendering.Initialized.Entities[h], "child": c.Rendering.Initialized.Entities[h+1]})
}

func TestMovingPresentationInterpolatesWithoutAdvancingActors(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprintf("scale%d", scale), func(t *testing.T) {
			c, h := movingTimingMovingController(t, scale, 60, "smooth")
			p := c.Rendering
			for i := 0; i < 20; i++ {
				advanceMoving(t, c, flight.StepDuration()+1)
			}
			child := p.Initialized.Entities[h+1]
			currentFound, previousFound := false, false
			var a, b [3]uint32
			for _, v := range p.presentation.previous.Instances {
				if v.ID == child.ID {
					a = [3]uint32{v.Pose.H0, v.Pose.H1, uint32(v.Pose.Height)}
					previousFound = true
				}
			}
			for _, v := range p.presentation.current.Instances {
				if v.ID == child.ID {
					b = [3]uint32{v.Pose.H0, v.Pose.H1, uint32(v.Pose.Height)}
					currentFound = true
				}
			}
			if !currentFound || !previousFound || b != [3]uint32{child.Pose.H0, child.Pose.H1, uint32(child.Pose.Height)} {
				t.Fatal("group child pose not published to actual draw registry", child.ID, currentFound, previousFound, a, b, child.Pose)
			}
			before := movingSampleAt(c, h)
			for _, alpha := range []float64{0, .25, .5, .75, 1} {
				p.presentation.alpha = alpha
				scene, _ := p.presentationInputs()
				found := false
				for _, v := range scene.Instances {
					if v.ID == child.ID {
						found = true
						if v.Pose.H0 != lerpPosition(a[0], b[0], alpha) || v.Pose.H1 != lerpPosition(a[1], b[1], alpha) {
							t.Fatal("display left previous/current simulation interval", alpha, a, b, v.Pose)
						}
					}
				}
				if !found {
					t.Fatal("rendered child identity lost")
				}
				p.Dirty = true
				c.Frame(false)
				if after := movingSampleAt(c, h); before != after {
					t.Fatal("interpolated draw changed authoritative group", before, after)
				}
			}
			writeTiming(t, fmt.Sprintf("display-scale%d.json", scale), map[string]any{"child_id": child.ID, "previous_pose": a, "current_pose": b, "tick": before.Tick, "alphas": []float64{0, .25, .5, .75, 1}, "authoritative_state_unchanged": true, "scope": "Actual presentation draw-instance registry and raster frames; no OS/GPU window"})
		})
	}
}
