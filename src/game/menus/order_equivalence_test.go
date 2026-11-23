package menus

import (
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/navigation"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type orderMechanicalSample struct {
	Steps                      uint64
	Groups                     map[string]string
	FollowerFlags              []byte
	FollowerKind, FollowerMode byte
	Pose                       any
}

// Frozen accepted preparation is copied, not regenerated. Recording output is
// optional and has no Store/career capability in this comparison. The separate
// connected pad run proves actual return/finalack; these samples never invent a
// return80 flag or infer a returned actor from coordinate proximity.
func TestOrderRecordingPresentationAndCompressionEquivalence(t *testing.T) {
	seed := mastFixture(t, 0, "pass")
	jFinishFade(t, seed)
	var launch filmLaunch
	if err := json.Unmarshal(seed.Rendering.filmRecorder.Film.Launch, &launch); err != nil {
		t.Fatal(err)
	}
	seed.AbandonFilmRecording()
	type variant struct {
		name             string
		record           bool
		scale, fps, rate int
		mode             string
	}
	variants := []variant{{"record1-legacy", true, 1, 10, 1, "stepped"}, {"off1-legacy", false, 1, 10, 1, "stepped"}, {"record2-smooth60", true, 2, 60, 1, "smooth"}, {"off2-smooth60", false, 2, 60, 1, "smooth"}, {"record1-compression2", true, 1, 10, 2, "stepped"}, {"off1-compression2", false, 1, 10, 2, "stepped"}, {"record2-compression2", true, 2, 60, 2, "smooth"}, {"off2-compression2", false, 2, 60, 2, "smooth"}}
	var want []orderMechanicalSample
	all := map[string]any{}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			final, err := cloneFilmFinal(&launch.Final)
			if err != nil {
				t.Fatal(err)
			}
			runtime := launch.Runtime
			runtime.SimulationRenderScale = v.scale
			runtime.PresentationFPS = v.fps
			runtime.PresentationMode = v.mode
			c := &FrontEnd{Office: seed.Office, Runtime: runtime, Screen: ScreenScene, filmIsolation: &filmSessionIsolation{Mode: "R1-equivalence"}, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
			if err = c.constructSimulation(final); err != nil {
				t.Fatal(err)
			}
			if v.record {
				c.FilmDirectory = t.TempDir()
				c.filmIsolation = nil
				c.beginFilmRecording(final)
				c.filmIsolation = &filmSessionIsolation{Mode: "R1-equivalence"}
				if c.Rendering.filmRecorder == nil {
					t.Fatal("normal recorder absent", c.Message)
				}
			}
			defer c.AbandonFilmRecording()
			p := c.Rendering
			act := func(a Actions) {
				t.Helper()
				if err := c.Handle(a); err != nil {
					t.Fatal(err)
				}
			}
			act(Actions{})
			act(Actions{ViewKey: 5, ViewShift: true})
			act(Actions{ViewKey: 1})
			act(Actions{FlightCommands: []int{2}})
			var samples []orderMechanicalSample
			snap := func() {
				allGroups := p.FilmCheckpoint()
				groups := map[string]string{}
				for _, k := range []string{"flight", "mission", "combat", "navigation", "environment", "pending-hit", "palette", "appearance", "orders-r1"} {
					groups[k] = allGroups[k]
				}
				e := p.Initialized.Entities[1]
				samples = append(samples, orderMechanicalSample{p.Flight.Steps, groups, append([]byte(nil), p.Initialized.ActorFlags...), e.Kind, e.ControlRaw[1], e.Pose})
			}
			advance := func(ticks int) {
				t.Helper()
				if v.rate == 2 {
					act(Actions{CompressionIncrease: true})
					if p.Flight.Compression.Rate() != 2 {
						t.Fatal("compression request failed")
					}
				}
				start := p.Flight.Steps
				total := time.Duration((int64(ticks)*flight.SourcePITDivisor*flight.NativeClocksPerUpdate*int64(time.Second) + flight.SourcePITHz*int64(v.rate) - 1) / (flight.SourcePITHz * int64(v.rate)))
				for elapsed := time.Duration(0); elapsed < total; {
					dt := min(time.Second/time.Duration(v.fps), total-elapsed)
					if err := c.AdvanceRendering(dt.Seconds()); err != nil {
						t.Fatal(err)
					}
					elapsed += dt
					if p.Flight.Failure != "" || c.Screen != ScreenScene {
						t.Fatalf("mechanical failure screen=%v %s", c.Screen, p.Flight.Failure)
					}
				}
				if p.Flight.Steps != start+uint64(ticks) {
					t.Fatalf("equal simulated time: %d ->%d want%d", start, p.Flight.Steps, start+uint64(ticks))
				}
				if v.rate == 2 {
					act(Actions{CompressionReset: true})
				}
				snap()
			}
			advance(16)
			act(Actions{GPSOpen: true})
			act(Actions{})
			act(Actions{GPSKey: "n"})
			if p.GPS == nil || p.GPS.Selected != 1 {
				t.Fatal("GPS recipient")
			}
			act(Actions{GPSKey: "f"})
			e := p.Initialized.Entities[1]
			x, y := navigation.HighPixel(navigation.Point{H0: e.Pose.H0 + 2048, H1: e.Pose.H1 + 2048})
			act(Actions{Inside: true, X: x, Y: y, Selector1: true})
			act(Actions{GPSKey: "f"})
			act(Actions{GPSKey: "n"})
			act(Actions{GPSKey: "h"})
			act(Actions{ViewKey: 1})
			act(Actions{})
			advance(16)
			act(Actions{WingmanOrder: "h"})
			advance(16)
			act(Actions{WingmanOrder: "h"})
			advance(16)
			act(Actions{WingmanOrder: "b"})
			advance(16)
			if want == nil {
				want = samples
			} else if !reflect.DeepEqual(want, samples) {
				for i := range samples {
					if !reflect.DeepEqual(want[i], samples[i]) {
						t.Fatalf("variant%s first differs sample%d tick%d: want%+v got%+v", v.name, i, samples[i].Steps, want[i], samples[i])
					}
				}
			}
			recordedOps := 0
			if p.filmRecorder != nil {
				if p.filmRecorder.Error != "" {
					t.Fatal(p.filmRecorder.Error)
				}
				recordedOps = p.filmRecorder.operationCount()
				if recordedOps == 0 {
					t.Fatal("recording path unused")
				}
			}
			all[v.name] = map[string]any{"recording": v.record, "scale": v.scale, "fps": v.fps, "rate": v.rate, "samples": samples, "operations": recordedOps, "scope": "actual cloned launch/input/order/scheduler; no ending or career capability; no staged return flag"}
		})
	}
	if out := os.Getenv("GS2000_R1_EQUIVALENCE_OUTPUT"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		b, _ := json.MarshalIndent(all, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "order-equivalence.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Log(fmt.Sprintf("%d same-launch variants compare five exact-tick mechanical snapshots", len(variants)))
}
