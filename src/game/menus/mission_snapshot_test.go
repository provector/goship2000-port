package menus

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gunship2000-assets/game/flightcontrol"
)

// Check private fields too. A new pointer on the captured graph must either be
// detached or have a documented, specific read-only owner.
func checkPointOwnership(t *testing.T, source, copied reflect.Value, path string) {
	t.Helper()
	if source.Type() != copied.Type() {
		t.Fatal(path, "different types")
	}
	switch copied.Kind() {
	case reflect.Interface:
		if !copied.IsNil() {
			checkPointOwnership(t, source.Elem(), copied.Elem(), path)
		}
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if copied.IsNil() || source.IsNil() {
			return
		}
		if source.Pointer() == copied.Pointer() {
			if copied.Kind() == reflect.Slice && copied.Cap() == 0 {
				return
			}
			switch {
			case strings.HasSuffix(path, ".Combat.assets"), strings.HasSuffix(path, ".Combat.city"),

				strings.HasSuffix(path, ".Map.Grid"), strings.HasSuffix(path, ".Map.Atlas"):
				return
			default:
				t.Fatal(path, "mutable storage shared by mission and snapshot")
			}
		}
		switch copied.Kind() {
		case reflect.Pointer:
			checkPointOwnership(t, source.Elem(), copied.Elem(), path)
		case reflect.Map:
			for _, k := range copied.MapKeys() {
				checkPointOwnership(t, source.MapIndex(k), copied.MapIndex(k), path+"[]")
			}
		case reflect.Slice:
			if copied.Type().Elem().Kind() >= reflect.Array {
				for i := 0; i < copied.Len(); i++ {
					checkPointOwnership(t, source.Index(i), copied.Index(i), path+"[]")
				}
			}
		}
	case reflect.Struct:
		for i := 0; i < copied.NumField(); i++ {
			checkPointOwnership(t, source.Field(i), copied.Field(i), path+"."+copied.Type().Field(i).Name)
		}
	case reflect.Array:
		if copied.Type().Elem().Kind() >= reflect.Array {
			for i := 0; i < copied.Len(); i++ {
				checkPointOwnership(t, source.Index(i), copied.Index(i), path+"[]")
			}
		}
	case reflect.Func:
		if !copied.IsNil() {
			t.Fatal(path, "observer still attached")
		}
	}
}

func TestSnapshotRemainsIndependentAcrossBranches(t *testing.T) {
	c := developmentScenarioController(t, "o-combat")
	if err := c.saveOrLoadPoint(); err != nil {
		t.Fatal(err)
	}
	mark := c.Rendering.sessionPoint
	s := mark.scene
	assertDetached := func() {
		t.Helper()
		checkPointOwnership(t, reflect.ValueOf(c.Rendering), reflect.ValueOf(s), "scene")
		if s.sessionPoint != nil || s.Renderer != nil || s.filmRecorder != nil || s.Flight.State != s.Initialized.PlayerFlight || s.Initialized.Targets != s.ResultTargets {
			t.Fatal("incorrect snapshot owner")
		}
	}
	for i := 0; i < 2; i++ {
		assertDetached()
		live := c.Rendering
		live.Flight.Systems.Fuel[0] -= 100
		live.avionicsReference[25] ^= 0xff
		live.Initialized.Stores.Ships[0].Fuel -= 20
		live.Initialized.RandomState++
		live.Flight.Combat.Score += 100
		if diff := compareFilmCheckpoint(mark.checkpoint, s.FilmCheckpoint()); diff != "" {
			t.Fatal("mutating branch touched saved state", diff)
		}
		if err := c.saveOrLoadPoint(); err != nil {
			t.Fatal(err)
		}
		if diff := compareFilmCheckpoint(mark.checkpoint, c.Rendering.FilmCheckpoint()); diff != "" {
			t.Fatal("restored state differs", diff)
		}
	}
	c.AbandonFilmRecording()
}

func TestSnapshotPreservesSurvivorAndPickupTransitions(t *testing.T) {
	// This historical fixture supplies an approach outside the recorded launch.
	// Exercise the scene copy at those boundaries without pretending the staged
	// positioning itself is a recordable input.
	c := developmentScenarioController(t, "n6-rescue")
	p := c.Rendering
	p.Flight.State.Pose.Height = 64
	p.Flight.State.HeightTarget = 64
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	var stored *RenderingPreview
	advance := func() {
		t.Helper()
		if err := c.AdvanceRendering(.101); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 80; i++ {
		advance()
		e := c.Rendering.Initialized.Entities[5]
		if e.Kind == 0x1b && e.Participation == 1 {
			var err error
			stored, err = c.Rendering.copyPointScene()
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if stored == nil {
		t.Fatal("fixture did not produce survivor")
	}
	var want []map[string]string
	for cycle := 0; cycle < 3; cycle++ {
		if cycle != 0 {
			v, err := stored.copyPointScene()
			if err != nil {
				t.Fatal(err)
			}
			v.Renderer = p.Renderer
			v.Frame = make([]byte, v.Scene.TargetPixels())
			c.Rendering, c.Mission.Initialized = v, v.Initialized
		}
		var seq []map[string]string
		for i := 0; i < 80; i++ {
			advance()
			seq = append(seq, c.Rendering.FilmCheckpoint())
			if c.Rendering.Initialized.Entities[5].Kind == 0x41 {
				break
			}
		}
		if c.Rendering.Initialized.Entities[5].Kind != 0x41 {
			t.Fatal("survivor was not picked up")
		}
		post, err := c.Rendering.copyPointScene()
		if err != nil || post.Initialized.Entities[5].Kind != 0x41 {
			t.Fatal("post-pickup state lost", err)
		}
		if cycle == 0 {
			want = seq
		} else if !reflect.DeepEqual(want, seq) {
			t.Fatal("rescue continuation differs")
		}
	}
	p.filmRecorder.Spool.Close()
}

func TestLoadedPointRetainsFuelReceiptAndCollectiveOwnership(t *testing.T) {
	c := developmentScenarioController(t, "o-short-campaign")
	jFinishFade(t, c)
	if err := c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	in := flightcontrol.LiveInput{Mode: "relative_height", Epoch: 1, Slider: true, Lever: 32768}
	in.Bound[2] = true
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	now := time.Duration(0)
	tick := func(keys ...int) {
		t.Helper()
		now += 110 * time.Millisecond
		if err := c.Handle(Actions{InputTime: now, Live: &in, FlightCommands: keys}); err != nil {
			t.Fatal(err)
		}
		if err := c.AdvanceRendering(.11); err != nil {
			t.Fatal(err)
		}
	}
	tick(0)
	tick(0)
	tick()
	tick()
	if !c.Rendering.Flight.collectiveOwner.Keyboard {
		t.Fatal("fixture lacks keyboard ownership")
	}
	if err := c.saveOrLoadPoint(); err != nil {
		t.Fatal(err)
	}
	fuel := c.Mission.ResultLaunch.fuel
	for cycle := 0; cycle < 2; cycle++ {
		in.Lever = 46000
		tick()
		tick()
		if c.Rendering.Flight.collectiveOwner.Keyboard {
			t.Fatal("deliberate lever movement failed to take control")
		}
		if fuel == c.Mission.ResultLaunch.fuel {
			t.Fatal("branch did not advance fuel receipts")
		}
		now += 2 * time.Minute // physical clock advances while no simulation input is admitted
		if err := c.saveOrLoadPoint(); err != nil {
			t.Fatal(err)
		}
		if fuel != c.Mission.ResultLaunch.fuel {
			t.Fatal("abandoned branch fuel receipts survived")
		}
		in.Lever = 32768
		tick()
		tick()
		if !c.Rendering.Flight.collectiveOwner.Keyboard {
			t.Fatal("parked stick stole keyboard after load")
		}
	}
	endFilm(t, c)
	verifyFilm(t, c)
}
