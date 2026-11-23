package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/render3d"
)

type presentationResult struct {
	authoritative, world, palette []byte
	steps, visual                 uint64
}

// Inputs use explicit normalized capture times, independently of the host draw
// batches delivering them. This tests actual accepted mission→flight→renderer
// ownership, not a separate physics interpreter or a replacement expected image.
func presentationRun(t *testing.T, mode string, fps int) presentationResult {
	t.Helper()
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	p.presentation.mode = mode
	p.resetPresentation()
	type event struct {
		at       time.Duration
		held     [4]bool
		commands []int
	}
	events := []event{
		{280 * time.Millisecond, [4]bool{false, false, true, false}, nil},
		{760 * time.Millisecond, [4]bool{true, false, true, false}, nil},
		{1110 * time.Millisecond, [4]bool{}, []int{5}}, // discrete MFD mode only
		{1480 * time.Millisecond, [4]bool{false, true, false, false}, nil},
		{1740 * time.Millisecond, [4]bool{}, nil},
	}
	feed := func(at time.Duration, held [4]bool, commands []int) {
		if err := c.Handle(Actions{InputTime: at, FlightHeldValid: true, SceneDirections: held, FlightCommands: commands}); err != nil {
			t.Fatal(err)
		}
	}
	feed(0, [4]bool{}, nil)
	eventIndex := 0
	var last time.Duration
	var held [4]bool
	for frame := 1; frame <= fps*2; frame++ {
		now := time.Duration(frame) * time.Second / time.Duration(fps)
		for eventIndex < len(events) && events[eventIndex].at <= now {
			e := events[eventIndex]
			held = e.held
			feed(e.at, held, e.commands)
			eventIndex++
		}
		feed(now, held, nil)
		if err := c.AdvanceRendering(float64(now-last) / float64(time.Second)); err != nil {
			t.Fatal(err)
		}
		last = now
		if c.Screen != ScreenScene || p.Flight.Failure != "" || p.Flight.State.Terminal != nil {
			t.Fatalf("unplanned stop:%s %s %+v", c.Screen, p.Flight.Failure, p.Flight.State.Terminal)
		}
		before, _ := json.Marshal(struct {
			Init, Env, Camera any
			Phase             uint64
		}{p.Initialized, p.Scene.Environment, p.ViewControls, p.Visual.Phase})
		c.Frame(false)
		world := append([]byte(nil), p.worldFrame...)
		p.Dirty = true // Force another actual rendering request from identical state.
		c.Frame(false)
		after, _ := json.Marshal(struct {
			Init, Env, Camera any
			Phase             uint64
		}{p.Initialized, p.Scene.Environment, p.ViewControls, p.Visual.Phase})
		if !bytes.Equal(before, after) || !bytes.Equal(world, p.worldFrame) {
			t.Fatal("extra draw changed authoritative state/canonical destination")
		}
		if p.Error != nil {
			t.Fatal(p.Error)
		}
	}
	state, err := json.Marshal(struct {
		Init, Flight, Systems, Camera, Env, Input any
		Phase                                     uint64
	}{p.Initialized, p.Flight.State, p.Flight.Systems, p.ViewControls, p.Scene.Environment, p.Flight.commandInput.Directions, p.Visual.Phase})
	if err != nil {
		t.Fatal(err)
	}
	return presentationResult{state, append([]byte(nil), p.worldFrame...), p.PaletteState.RGB6(), p.Flight.Steps, p.Visual.Phase}
}

func TestActualMissionPresentationSchedules(t *testing.T) {
	var baseline presentationResult
	for _, mode := range []string{"stepped", "smooth"} {
		for _, fps := range []int{30, 60, 144} {
			t.Run(fmt.Sprintf("%s-%d", mode, fps), func(t *testing.T) {
				got := presentationRun(t, mode, fps)
				t.Logf("physics_steps=%d visual_steps=%d authoritative_sha256=%x canonical_world_sha256=%x palette_sha256=%x", got.steps, got.visual, sha256.Sum256(got.authoritative), sha256.Sum256(got.world), sha256.Sum256(got.palette))
				if got.visual != 24 {
					t.Fatalf("two seconds must advance24 declared12Hz visual updates, got%d", got.visual)
				}
				if baseline.authoritative == nil {
					baseline = got
					return
				}
				if !bytes.Equal(baseline.authoritative, got.authoritative) {
					t.Fatalf("normalized physics/camera/environment outcome changed, steps%d/%d visual%d/%d state hashes%x/%x", baseline.steps, got.steps, baseline.visual, got.visual, sha256.Sum256(baseline.authoritative), sha256.Sum256(got.authoritative))
				}
				if !bytes.Equal(baseline.world, got.world) {
					t.Fatal("canonical indexed destination depends on draw schedule/presentation mode")
				}
				if !bytes.Equal(baseline.palette, got.palette) {
					t.Fatal("active palette depends on draw schedule/presentation mode")
				}
			})
		}
	}
}

func TestActualMissionIntermediatePoseAndDiscreteInputs(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	p.presentation.mode = "smooth"
	if err := c.AdvanceRendering(.15); err != nil {
		t.Fatal(err)
	}
	if !p.presentation.ready || p.Flight.Steps == 0 || p.presentation.alpha <= 0 || p.presentation.alpha >= 1 {
		t.Fatal("missing interpolation interval", p.presentation.alpha)
	}
	scene, in := p.presentationInputs()
	a, b := p.presentation.previous, p.presentation.current
	if a.Camera == b.Camera {
		t.Fatal("fixture did not move; interpolation unexercised")
	}
	if scene.Camera == a.Camera || scene.Camera == b.Camera {
		t.Fatal("intermediate pose snapped to a logical endpoint")
	}
	if in.Heading != lerpAngle(a.Cockpit.Heading, b.Cockpit.Heading, p.presentation.alpha) {
		t.Fatal("camera/cockpit interpolation fraction mismatch")
	}
	if in.Stations != p.Cockpit.Stations || in.Defense != p.Cockpit.Defense || in.Damage != p.Cockpit.Damage || in.PrimaryMFD != p.Cockpit.PrimaryMFD || in.SystemFlags != p.Cockpit.SystemFlags {
		t.Fatal("discrete values interpolated or lost")
	}
	before := *p.Flight.State
	canonical := append([]byte(nil), p.worldFrame...)
	geometry := map[string][][3]int16{}
	for id := range p.Renderer.Assets.Geometries {
		for _, vertex := range p.Visual.SnapshotVertices(id) {
			geometry[id] = append(geometry[id], [3]int16(vertex))
		}
	}
	for _, alpha := range []float64{.1, .3, .7, .9} {
		p.presentation.alpha = alpha
		p.Dirty = true
		c.Frame(false)
		if p.Error != nil {
			t.Fatal(p.Error)
		}
	}
	if *p.Flight.State != before || !bytes.Equal(canonical, p.worldFrame) {
		t.Fatal("intermediate draws changed physics/canonical world")
	}
	for id, before := range geometry {
		after := p.Visual.SnapshotVertices(id)
		if len(before) != len(after) {
			t.Fatal("drawing changed shared geometry length", id)
		}
		for i, v := range after {
			if [3]int16(v) != before[i] {
				t.Fatal("drawing changed shared geometry vertex", id, i)
			}
		}
	}
}

func TestActualMissionPresentationModalViewAndReentry(t *testing.T) {
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	p := c.Rendering
	p.presentation.mode = "smooth"
	if err := c.AdvanceRendering(.15); err != nil {
		t.Fatal(err)
	}
	state := *p.Flight.State
	camera := p.Scene.Camera
	canonical := append([]byte(nil), p.worldFrame...)
	act(t, c, Actions{Quit: true})
	if err := c.AdvanceRendering(.2); err != nil {
		t.Fatal(err)
	}
	if *p.Flight.State != state || p.Scene.Camera != camera {
		t.Fatal("modal advanced flight/camera")
	}
	act(t, c, Actions{Text: []rune{'N'}, AnyKey: true})
	if c.Screen != ScreenScene || *p.Flight.State != state || p.Scene.Camera != camera {
		t.Fatal("quit cancel changed view/state")
	}
	if p.presentation.previous.Camera != p.presentation.current.Camera || p.presentation.alpha != 1 {
		t.Fatal("modal reset retained stale interpolation endpoints")
	}
	c.Frame(false)
	if !bytes.Equal(canonical, p.worldFrame) {
		t.Fatal("modal cancellation advanced canonical destination without a logical update")
	}
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	} // explicit release after modal
	if err := c.Handle(Actions{ViewKey: 3}); err != nil {
		t.Fatal(err)
	}
	if p.presentation.previous.Camera != p.presentation.current.Camera || p.presentation.alpha != 1 {
		t.Fatal("view switch failed to snap history")
	}
	if *p.Flight.State != state {
		t.Fatal("view changed authoritative flight")
	}
	act(t, c, Actions{FlightPause: true})
	if err := c.AdvanceRendering(.2); err != nil {
		t.Fatal(err)
	}
	if *p.Flight.State != state {
		t.Fatal("pause changed authoritative flight")
	}
	act(t, c, Actions{AnyKey: true})
	act(t, c, Actions{})
	if p.presentation.previous.Camera != p.presentation.current.Camera || p.presentation.alpha != 1 {
		t.Fatal("resume retained stale history")
	}
	mission := c.Mission
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || c.Mission != mission {
		t.Fatal("QoL return lost mission")
	}
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Rendering == p || c.Rendering.presentation.previous.Camera != c.Rendering.presentation.current.Camera {
		t.Fatal("reentry retained previous sortie history")
	}
	if !reflect.DeepEqual(c.Rendering.Scene.Camera, c.Rendering.presentation.current.Camera) {
		t.Fatal("reentry snapshot not current view")
	}
}

func TestPresentationHandAngleWrapWithActualTables(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	p := c.Rendering
	p.presentation.mode = "smooth"
	p.presentation.alpha = .5
	p.presentation.previous.Camera.Angles[1] = 0xff00
	p.presentation.current.Camera.Angles[1] = 0x100
	p.presentation.previous.Cockpit.Heading = 0xff00
	p.presentation.current.Cockpit.Heading = 0x100
	scene, in := p.presentationInputs()
	if scene.Camera.Angles[1] != 0 || in.Heading != 0 {
		t.Fatal("wrapped angle took long path")
	}
	if flight.NativeClockPolicy != "adlib-baseline-six-clock-quantum-v1" {
		t.Fatal("reference physics policy changed")
	}
	if scene.Camera.Pose != (render3d.Pose{H0: p.Scene.Camera.H0, H1: p.Scene.Camera.H1, Height: p.Scene.Camera.Height}) {
		t.Fatal("angle-only supplied case moved camera")
	}
}
