package menus

import (
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type controllerBackend struct{ s controllers.Snapshot }

func (b *controllerBackend) Poll() (controllers.Snapshot, error) { return b.s, nil }
func (b *controllerBackend) Close()                              {}
func controllerDevice(c *FrontEnd) (*gameController, *controllerBackend) {
	d := controllers.Device{Instance: 9, Backend: "test", Name: "test", Axes: 4, Buttons: 16, Hats: 1}
	p := controllers.Profile{ID: "p", Device: d, CollectiveMode: "absolute", Bindings: map[string]controllers.Binding{"flight.roll": {Kind: "axis", Axis: 0}, "flight.pitch": {Kind: "axis", Axis: 1}, "flight.collective": {Kind: "axis", Axis: 2}, "menu.selector1": {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 0}}, "menu.selector2": {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 1}}, "nav.open": {Kind: "control", Control: &controllers.Control{Kind: "button", Index: 2}}}}
	b := &controllerBackend{s: controllers.Snapshot{Devices: []controllers.Device{d}, States: map[int32]controllers.State{9: {Axes: []float64{0, 0, -1, 0}, Buttons: make([]bool, 16)}}}}
	g := &gameController{live: controllers.NewLive(controllers.Config{Version: 2, Selected: "p", Profiles: map[string]controllers.Profile{"p": p}}, b), previous: map[string]bool{}}
	return g, b
}
func TestEveryEditableCommandHasExplicitOwner(t *testing.T) {
	seen := map[string]bool{}
	for _, v := range controllerCommands {
		if seen[v.ID] {
			t.Fatal("duplicate", v.ID)
		}
		seen[v.ID] = true
		a, ok := controllers.MappingAction(v.ID)
		if !ok || a.Status == "deferred" {
			t.Fatal(v)
		}
		yes, _ := controllers.AdvancedEligibility(a)
		if !yes {
			t.Fatal("hidden dispatch", v)
		}
	}
	count := 0
	for _, a := range controllers.AdvancedActions() {
		if a.Status == "deferred" {
			continue
		}
		count++
		if a.Axis {
			continue
		}
		if _, ok := controllers.DirectionByID(a.ID); ok {
			continue
		}
		if !seen[a.ID] {
			t.Fatal("missing command", a.ID)
		}
	}
	// This asserts the current catalogue, not a historical film's input schema.
	// R4 adds three source-documented MFD/editor consumers, Last Message and
	// source-dispatched jettison to Q3's154 editable rows/143 discrete owners.
	if count+len(controllers.AxisDirections) != 159 || len(controllerCommands) != 148 {
		t.Fatal(count, len(controllerCommands))
	}
}
func TestKeyboardOnlyYawChordAndReleaseOrdering(t *testing.T) {
	c := &FrontEnd{Screen: ScreenScene, Office: &Office{}}
	g := &gameController{}
	b := g.Merge(c, Buttons{SceneDirections: [4]bool{false, true}, Right: true}, .1, 0, 0, true, nil)
	if b.SceneDirections[1] || b.Right || !b.Live.Digital[3][1] || b.Live.Digital[1][1] {
		t.Fatal("chord leaked", b)
	}
	b = g.Merge(c, Buttons{SceneDirections: [4]bool{false, true}, Right: true}, .1, 0, 0, false, nil)
	if b.SceneDirections[1] || b.Live.Digital[3][1] {
		t.Fatal("Alt released first", b)
	}
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	b = g.Merge(c, Buttons{SceneDirections: [4]bool{false, true}, Right: true}, .1, 0, 0, false, nil)
	if !b.Live.Digital[1][1] || b.Live.Digital[3][1] {
		t.Fatal("fresh bank", b)
	}
	c.Screen = ScreenOffice
	b = g.Merge(c, Buttons{SceneDirections: [4]bool{false, true}, Right: true}, .1, 0, 0, true, nil)
	if !b.Right || !b.SceneDirections[1] {
		t.Fatal("navigation changed", b)
	}
}
func TestSourcesPointerAndContextRearm(t *testing.T) {
	c := &FrontEnd{Screen: ScreenOffice, Office: &Office{PointerX: 160, PointerY: 90}}
	g, b := controllerDevice(c)
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	raw := b.s.States[9]
	raw.Buttons[0] = true
	raw.Axes[0] = .5
	b.s.States[9] = raw
	latch := inputLatch{}
	merged := g.Merge(c, Buttons{LeftMouse: true}, .1, 0, 0, false, nil)
	a := latch.Read(merged)
	if !a.PointerPressed || merged.X <= 160 || !merged.Inside {
		t.Fatal(merged, a)
	}
	raw.Buttons[0] = false
	b.s.States[9] = raw
	a = latch.Read(g.Merge(c, Buttons{LeftMouse: true}, .1, 0, 0, false, nil))
	if a.PointerReleased || a.PointerPressed {
		t.Fatal("controller release lost mouse", a)
	}
	a = latch.Read(g.Merge(c, Buttons{}, .1, 0, 0, false, nil))
	if !a.PointerReleased {
		t.Fatal(a)
	}
	raw.Buttons[0] = true
	b.s.States[9] = raw
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	c.Screen = ScreenScene
	merged = g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	if merged.WeaponTrigger {
		t.Fatal("held selector across screen", merged)
	}
	raw.Buttons[0] = false
	b.s.States[9] = raw
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	raw.Buttons[0] = true
	b.s.States[9] = raw
	merged = g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	if !merged.WeaponTrigger {
		t.Fatal("no fresh trigger", merged)
	}
	b.s.Devices = nil
	b.s.States = nil
	merged = g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	if merged.WeaponTrigger || merged.Live.Bound[0] {
		t.Fatal("disconnect held", merged)
	}
}
func TestCursorWallTimePartitionAndStationaryMouse(t *testing.T) {
	for _, fps := range []int{10, 60} {
		c := &FrontEnd{Screen: ScreenOffice, Office: &Office{PointerX: 10, PointerY: 90}}
		g, b := controllerDevice(c)
		g.Merge(c, Buttons{}, 0, 0, 0, false, nil)
		raw := b.s.States[9]
		raw.Axes[0] = 1
		b.s.States[9] = raw
		for i := 0; i < fps; i++ {
			v := g.Merge(c, Buttons{X: 1, Y: 1, Inside: true}, 1/float64(fps), 0, 0, false, nil)
			c.Office.PointerX, c.Office.PointerY = v.X, v.Y
		}
		if c.Office.PointerX != 130 {
			t.Fatal(fps, c.Office.PointerX)
		}
		raw.Axes[0] = 0
		b.s.States[9] = raw
		v := g.Merge(c, Buttons{X: 1, Y: 1, Inside: true}, .1, 0, 0, false, nil)
		if v.X != 130 {
			t.Fatal("stationary mouse snapped", v.X)
		}
		v = g.Merge(c, Buttons{X: 1, Y: 1, Inside: true}, .1, -7, 0, false, nil)
		if v.X != 123 {
			t.Fatal("mouse takeover", v.X)
		}
	}
}
func TestContinuousInputFilmStrictAndPresentationPartitions(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	// Genuine current simulation producers, controlled input stream; no flight
	// physics or checkpoint setters. This is a model witness, not an OS flight.
	for i := 0; i < 40; i++ {
		in := &flightcontrol.LiveInput{Epoch: 1, Axes: [4]int16{int16(i % 5 * 7), int16(i % 3 * 11), 32, 17}, Bound: [4]bool{true, true, true, true}}
		if i > 20 {
			in.Absolute = true
			in.Lever = uint16((i - 20) * 3000)
		}
		if e := c.Handle(Actions{Live: in, InputTime: time.Duration(i) * 100 * time.Millisecond}); e != nil {
			t.Fatal(e)
		}
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	if e := c.openGPS(); e != nil {
		t.Fatal(e)
	}
	c.handleFlight(Actions{})
	if e := c.Handle(Actions{GPSKey: "space"}); e != nil {
		t.Fatal(e)
	}
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	endFilm(t, c)
	f := verifyFilm(t, c)
	if f.Header.InputSchema != replay.R1InputCompatibility {
		t.Fatal(f.Header)
	}
	live := false
	for _, o := range f.Operations {
		if strings.Contains(string(o.Payload), `"Live"`) {
			live = true
		}
	}
	if !live {
		t.Fatal("continuous operations missing")
	}
	for _, fps := range []int{10, 60} {
		opt := configuration.Defaults()
		q, e := NewFilmPlayback(c.Office.Assets, f, opt)
		if e != nil {
			t.Fatal(e)
		}
		v := NewReplayViewer(q)
		// Each transport owns its playback/verification lifecycle. The source front
		// end can still be finishing a rejected-result verification here.
		host, e := NewFrontEnd(c.Office.Assets, c.Store)
		if e != nil {
			t.Fatal(e)
		}
		host.Screen = ScreenReplay
		host.Viewer = v
		for frame := 0; !q.Done; frame++ {
			if frame > 120*f.OperationCount()+120 {
				t.Fatalf("replay did not finish at %d FPS: cursor=%d/%d screen=%s stopped=%t terminal-ack=%t verifying-result=%t message=%q divergence=%q", fps, q.Cursor, f.OperationCount(), host.Screen, v.Stopped, q.Preview().Flight.terminalAwaitingAck, host.rejectedCheck != nil, host.Message, q.Divergence)
			}
			if e = host.AdvanceReplay(1 / float64(fps)); e != nil {
				t.Fatal(fps, e)
			}
		}
	}
}

func TestEveryDiscreteTokenReachesItsExistingButtonCarrier(t *testing.T) {
	for _, cmd := range controllerCommands {
		t.Run(cmd.ID, func(t *testing.T) {
			var b Buttons
			applyControllerCommand(&b, cmd.Context, cmd.Token, true, nil)
			if reflect.DeepEqual(b, Buttons{}) {
				t.Fatal("no existing carrier", cmd)
			}
		})
	}
}
func TestMissingInvalidAndNoneControlsAreOptionalReadOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, controllers.Filename)
	for _, content := range []string{"", `{"version":999}`, `{"version":2,"profiles":{}}`} {
		if content != "" {
			if e := os.WriteFile(p, []byte(content), 0600); e != nil {
				t.Fatal(e)
			}
		}
		g := openGameController(dir)
		g.Close()
		got, e := os.ReadFile(p)
		if content == "" {
			if !os.IsNotExist(e) {
				t.Fatal("game generated controls", e)
			}
		} else if string(got) != content {
			t.Fatal("rewrote invalid or None controls")
		}
	}
}
func TestLogicalSamplingAcrossFPSAndCompressedTicks(t *testing.T) {
	var want flightcontrol.LiveState
	for _, fps := range []int{10, 60} {
		q := flightCommandTimeline{}
		q.ResetTimeline()
		q.LogicalSampling = true
		state := flightcontrol.LiveState{}
		a := flightcontrol.Axes{}
		collective := int16(10)
		in := flightcontrol.LiveInput{Epoch: 1, Axes: [4]int16{27, 54, 32, 17}, Bound: [4]bool{true, true, true, true}}
		// All source updates in a compression batch consume the latest accepted
		// sample, not one synthetic host deflection per rendered picture.
		ticks := 0
		for frame := 0; frame < fps; frame++ {
			q.Sample(Actions{Live: &in, InputTime: time.Duration(frame) * time.Second / time.Duration(fps)})
			n := 60 / fps
			for i := 0; i < n; i++ {
				q.NextLogical()
				collective = state.Apply(*q.Live, &a, collective, false)
				ticks++
			}
		}
		if ticks != 60 || a.X != 54 || a.Y != 27 || collective != 25 {
			t.Fatal(fps, ticks, a, collective)
		}
		if fps == 10 {
			want = state
		} else if state != want {
			t.Fatal("partition changed input state", state, want)
		}
	}
}
func TestContinuousSchemaRejectsTamperAndHistoricalInjection(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	c.Handle(Actions{Live: &flightcontrol.LiveInput{Epoch: 1}})
	c.advanceFlight(.2)
	endFilm(t, c)
	f := verifyFilm(t, c)
	var op *replay.Operation
	for i := range f.Operations {
		if f.Operations[i].Checkpoint["controls-q3"] != "" {
			op = &f.Operations[i]
			break
		}
	}
	if op == nil {
		t.Fatal("no new input owner")
	}
	old := op.Checkpoint["controls-q3"]
	op.Checkpoint["controls-q3"] = strings.Repeat("0", 64)
	q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if e == nil {
		for !q.Done && e == nil {
			e = q.Step()
		}
	}
	if e == nil {
		t.Fatal("input checkpoint tamper accepted")
	}
	op.Checkpoint["controls-q3"] = old
	f.Header.InputSchema = replay.P6InputCompatibility
	if _, e = NewFilmPlayback(c.Office.Assets, f, configuration.Defaults()); e == nil {
		t.Fatal("new input injected into old profile")
	}
}

func TestAggregateTriggerDoesNotRetriggerFromAnotherHeldSource(t *testing.T) {
	l := inputLatch{}
	live := &flightcontrol.LiveInput{Epoch: 1}
	a := l.Read(Buttons{Live: live, WeaponTrigger: true})
	if !a.WeaponTriggerPressed {
		t.Fatal(a)
	}
	a = l.Read(Buttons{Live: live, WeaponTrigger: true, WeaponTriggerPressed: true})
	if a.WeaponTriggerPressed || !a.WeaponTriggerHeld {
		t.Fatal("second held source retriggered", a)
	}
	a = l.Read(Buttons{Live: live, WeaponTrigger: true})
	if !a.WeaponTriggerHeld {
		t.Fatal(a)
	}
	l.Read(Buttons{Live: live})
	a = l.Read(Buttons{Live: live, WeaponTriggerPressed: true})
	if !a.WeaponTriggerPressed {
		t.Fatal("sub-poll tap lost", a)
	}
}

func TestSwapControllerChannelsPreservesKeyboardRoles(t *testing.T) {
	c := &FrontEnd{Screen: ScreenScene, Office: &Office{}}
	g, b := controllerDevice(c)
	p := g.live.Config.Profiles["p"]
	p.Bindings["flight.yaw"] = controllers.Binding{Kind: "axis", Axis: 3}
	p.AxisButtons = map[string]controllers.Binding{"flight.yaw": {Kind: "pair", Positive: &controllers.Control{Kind: "button", Index: 5}}}
	g.live.Config.Profiles["p"] = p
	g.swapYawRoll = true
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	raw := b.s.States[9]
	raw.Axes[0] = .5
	raw.Axes[3] = -.75
	raw.Buttons[5] = true
	b.s.States[9] = raw
	v := g.Merge(c, Buttons{SceneDirections: [4]bool{true}}, .1, 0, 0, false, nil)
	if v.Live.Axes[1] != -95 || v.Live.Axes[3] != 64 || !v.Live.Digital[1][1] || !v.Live.Digital[1][0] {
		t.Fatal("channel swap", v.Live)
	}
	v = g.Merge(c, Buttons{SceneDirections: [4]bool{false, true}}, .1, 0, 0, true, nil)
	if !v.Live.Digital[3][1] || v.SceneDirections[1] {
		t.Fatal("keyboard yaw swapped", v)
	}
}

func TestMappedShiftViewDoesNotLeakIntoCollectiveOrAnotherView(t *testing.T) {
	b := Buttons{Live: &flightcontrol.LiveInput{Epoch: 1}, FlightKeys: [11]bool{true}}
	applyControllerCommand(&b, "flight", "shiftview5", true, nil)
	l := inputLatch{}
	a := l.Read(b)
	if !a.ViewShift || a.ViewKey != 5 || a.CollectiveShift || !a.CollectiveShiftValid || a.FlightCommands[0] != 0 {
		t.Fatal(a)
	}
	q := flightCommandInput{}
	q.SampleAt(0, a)
	q.NextAt(0)
	if q.shift {
		t.Fatal("virtual view modifier leaked")
	}
	b.FunctionKeys[0] = true
	a = l.Read(b)
	if a.ViewKey != 1 || a.ViewShift {
		t.Fatal("held Shift+F5 changed a fresh F1", a)
	}
}

func TestActualGPSHeldSelectorCannotFireClickOrBankAcrossTransition(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	r := c.Rendering.Flight
	g, b := controllerDevice(c)
	p := g.live.Config.Profiles["p"]
	p.Bindings["gps.cancel"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 3}}
	g.live.Config.Profiles["p"] = p
	latch := inputLatch{}
	dispatch := func() {
		t.Helper()
		raw := g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
		previous := latch.previous
		a := latch.Read(raw)
		a = normalizeSceneSelectors(a, raw.LeftMouse && !previous.LeftMouse, raw.RightMouse && !previous.RightMouse, true)
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
	}
	dispatch()
	dispatch()
	state := b.s.States[9]
	state.Buttons[0] = true
	state.Buttons[2] = true
	b.s.States[9] = state
	dispatch()
	if c.Rendering.GPS == nil {
		t.Fatal("mapped GPS open failed")
	}
	pose := r.State.Pose
	steps := r.Steps
	dispatch()
	if c.Rendering.GPS == nil || r.State.Pose != pose || r.Steps != steps || r.commandInput.Trigger {
		t.Fatal("held trigger became GPS click or flight update")
	}
	state.Buttons[2] = false
	state.Buttons[3] = true
	b.s.States[9] = state
	dispatch()
	if c.Rendering.GPS != nil {
		t.Fatal("mapped GPS cancel did not exit")
	}
	dispatch()
	if r.commandInput.Trigger {
		t.Fatal("held selector fired on exit")
	}
	state.Buttons[0] = false
	state.Buttons[3] = false
	b.s.States[9] = state
	dispatch()
	state.Buttons[0] = true
	b.s.States[9] = state
	dispatch()
	r.commandInput.NextAt(time.Second)
	if !r.commandInput.Trigger {
		t.Fatal("fresh flight trigger did not rearm")
	}
}

func TestFlightSwapNeverSwapsMenuCyclicPointer(t *testing.T) {
	c := &FrontEnd{Screen: ScreenOffice, Office: &Office{PointerX: 100, PointerY: 90}}
	g, b := controllerDevice(c)
	g.swapYawRoll = true
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	raw := b.s.States[9]
	raw.Axes[0] = 1
	b.s.States[9] = raw
	v := g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	if v.X != 112 {
		t.Fatal("flight swap stole horizontal stick from menus", v.X)
	}
}

func TestEditorChoiceSelectorTwoAndRightMouseCancel(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		c := controlsEditor(t)
		before := string(filmJSON(c.Editor.Document))
		if e := c.editorArm("defender", 0, 0); e != nil {
			t.Fatal(e)
		}
		if c.Editor.Stage != "option" {
			t.Fatal(c.Editor.Stage)
		}
		a := Actions{Selector2: true}
		if mouse {
			a.Inside = true
			a.X, a.Y = c.Editor.CursorX, c.Editor.CursorY
			a.PointerPressed = true
			a.PointerRightHeld = true
			a.PointerStateValid = true
		}
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
		if c.Editor.Stage != "edit" || c.Editor.Held || string(filmJSON(c.Editor.Document)) != before {
			t.Fatal("popup cancel mutated draft", mouse, c.Editor.Stage)
		}
	}
}

func TestRetainedBackspacePreviewCycleRemainsReachable(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	e.Formation = true
	for cat := 1; cat <= 10; cat++ {
		if err := c.editorArm("defender", 0, cat); err != nil {
			continue
		}
		for i := range e.Options {
			e.Option = i
			rows, err := c.editorPreviewRows()
			if err != nil || len(rows) < 2 {
				continue
			}
			rect := e.popupArea(i, len(e.Options))
			e.CursorX, e.CursorY = rect[0]+2, rect[1]+2
			old := e.ComponentPreview
			if err := c.Handle(Actions{Backspace: true}); err != nil {
				t.Fatal(err)
			}
			if e.Stage != "option" || e.ComponentPreview != (old+1)%len(rows) {
				t.Fatal("existing keyboard preview cycle lost", e.Stage, e.ComponentPreview)
			}
			return
		}
	}
	t.Fatal("required multi-component source formation fixture unavailable")
}

func TestPhysicalSelectorsRemainIndependentOfKeyboardYawModifier(t *testing.T) {
	b := Buttons{Live: &flightcontrol.LiveInput{Epoch: 1}}
	applyControllerCommand(&b, "flight", "selector1", true, nil)
	applyControllerCommand(&b, "flight", "selector2", true, nil)
	l := inputLatch{}
	a := normalizeSceneSelectors(l.Read(b), false, false, false)
	if !a.WeaponTriggerHeld || !a.Selector2 {
		t.Fatal("keyboard Alt suppressed physical selector", a)
	}
	a = normalizeSceneSelectors(l.Read(b), false, false, false)
	if a.Selector2 {
		t.Fatal("held controller target repeated")
	}
}

func TestTargetSourcesShareOnePressAndReleaseEpoch(t *testing.T) {
	l := inputLatch{}
	live := &flightcontrol.LiveInput{Epoch: 1}
	read := func(b Buttons) Actions { b.Live = live; return normalizeSceneSelectors(l.Read(b), false, false, true) }
	if !read(Buttons{ControllerTarget: true}).Selector2 {
		t.Fatal("missing controller target")
	}
	if read(Buttons{ControllerTarget: true, Backspace: true}).Selector2 {
		t.Fatal("keyboard duplicated held target")
	}
	if read(Buttons{Backspace: true}).Selector2 {
		t.Fatal("controller release repeated target")
	}
	read(Buttons{})
	if !read(Buttons{Backspace: true}).Selector2 {
		t.Fatal("fresh keyboard target lost")
	}
	if read(Buttons{Backspace: true, ControllerTarget: true}).Selector2 {
		t.Fatal("controller duplicated keyboard target")
	}
}

func TestFocusLossReleasesHoldsWithoutRestoringOldMouseCoordinates(t *testing.T) {
	c := &FrontEnd{Screen: ScreenOffice, Office: &Office{PointerX: 130, PointerY: 90}}
	g, b := controllerDevice(c)
	g.Merge(c, Buttons{}, .1, 0, 0, false, nil)
	g.pointerOwned = true
	v := g.Merge(c, Buttons{FlightFocusLost: true, X: 5, Y: 6, LeftMouse: true}, .1, 0, 0, false, nil)
	if v.X != 130 || v.Y != 90 || v.Inside || v.LeftMouse {
		t.Fatal("focus loss changed pointer or held input", v)
	}
	c.Office.PointerX, c.Office.PointerY = v.X, v.Y
	raw := b.s.States[9]
	raw.Axes[0] = 0
	b.s.States[9] = raw
	v = g.Merge(c, Buttons{X: 5, Y: 6, Inside: true}, .1, 0, 0, false, nil)
	if v.X != 130 || v.Y != 90 {
		t.Fatal("stationary mouse snapped after focus", v)
	}
}

func TestLiveCollectiveFollowsFlightModel(t *testing.T) {
	for _, configured := range []string{"relative_height", "relative", "absolute"} {
		for _, source := range []string{"centered", "slider"} {
			for _, invert := range []bool{false, true} {
				c := &FrontEnd{Screen: ScreenScene, Office: &Office{}, Rendering: &RenderingPreview{Initialized: &gamestate.InitializedMission{}}}
				g, b := controllerDevice(c)
				p := g.live.Config.Profiles["p"]
				p.CollectiveMode, p.CollectiveSource = configured, source
				bind := p.Bindings["flight.collective"]
				bind.Invert = invert
				p.Bindings["flight.collective"] = bind
				g.live.Config.Profiles["p"] = p
				for _, realistic := range []bool{false, true, false} {
					c.Rendering.Initialized.Switches = 0
					want := configured
					if realistic {
						c.Rendering.Initialized.Switches = 4
						want = "absolute"
					}
					for _, axis := range []float64{0, -1, 0, 1} {
						b.s.States[9].Axes[2] = axis
						in := g.Merge(c, Buttons{}, .016, 0, 0, false, nil).Live
						if in.Mode != want || in.Absolute != (want == "absolute") || in.Slider != (source == "slider") || !in.Bound[2] {
							t.Fatalf("%s / %s realistic=%t raw=%v: %+v", configured, source, realistic, axis, in)
						}
						if want == "absolute" {
							value := axis
							if invert {
								value = -axis
							}
							expected := uint16(math.Round((value + 1) * 32767.5))
							if in.Lever != expected {
								t.Fatalf("lever %d want %d", in.Lever, expected)
							}
						}
					}
				}
				if g.live.Config.Profiles["p"].CollectiveMode != configured {
					t.Fatal("profile rewritten")
				}
			}
		}
	}
}

func TestRealisticControllerContinuationAfterMissionPoint(t *testing.T) {
	c := renderingController(t)
	c.simulationProfile = ""
	c.Mission.Settings.FlightModel = 1
	startRendering(t, c)
	jFinishFade(t, c)
	if c.Rendering.Initialized.Switches&4 == 0 {
		t.Fatal("fixture requires realistic flight")
	}
	g, b := controllerDevice(c)
	profile := g.live.Config.Profiles["p"]
	profile.CollectiveMode = "relative_height"
	g.live.Config.Profiles["p"] = profile
	if err := c.Handle(Actions{FlightCommands: []int{2}}); err != nil {
		t.Fatal(err)
	}
	step := func(axis float64) {
		t.Helper()
		b.s.States[9].Axes[2] = axis
		in := g.Merge(c, Buttons{}, .1, 0, 0, false, nil).Live
		if in.Mode != "absolute" || !in.Absolute {
			t.Fatal("realistic input mode", in)
		}
		if err := c.Handle(Actions{Live: in}); err != nil {
			t.Fatal(err)
		}
		if err := c.AdvanceRendering(.1); err != nil {
			t.Fatal(err)
		}
	}
	step(0)
	useComparedMissionPoint(t, c)
	step(.5)
	useComparedMissionPoint(t, c)
	step(-.5)
	step(.75)
	endFilm(t, c)
	verifyFilm(t, c)
}

func TestHoverCollectiveContinuationThroughMissionPoint(t *testing.T) {
	for _, tc := range []struct {
		name, mode string
		realistic  bool
	}{
		{"easy_height", "relative_height", false}, {"easy_absolute", "absolute", false}, {"easy_relative", "relative", false}, {"realistic_absolute", "absolute", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := renderingController(t)
			c.simulationProfile = ""
			if tc.realistic {
				c.Mission.Settings.FlightModel = 1
			} else {
				c.Mission.Settings.FlightModel = 0
			}
			startRendering(t, c)
			jFinishFade(t, c)
			f := c.Rendering.Flight.State
			initial := FilmApproach{H0: f.Pose.H0, H1: f.Pose.H1, AboveSupport: 800, Rotor: 1, Stage: 128, Hover: 1, HeightControl: 1}
			if e := c.ApplyFilmInitialAdjustment(FilmInitialAdjustment{Label: "Hover control handover", Approach: &initial}); e != nil {
				t.Fatal(e)
			}
			input := flightcontrol.LiveInput{Mode: tc.mode, Epoch: 1, Bound: [4]bool{false, false, true, true}, Absolute: tc.mode == "absolute"}
			tick := func(command []int) {
				t.Helper()
				if e := c.Handle(Actions{Live: &input, FlightCommands: command}); e != nil {
					t.Fatal(e)
				}
				if e := c.AdvanceRendering(.12); e != nil {
					t.Fatal(e)
				}
			}
			tick(nil)
			useComparedMissionPoint(t, c)
			for i := 0; i < 4; i++ {
				tick([]int{0})
			}
			useComparedMissionPoint(t, c)
			s := c.Rendering.Flight.State
			tick(nil) // Admit the first post-restore input clock boundary.
			heading := s.Pose.Angles[1]
			tick([]int{0})
			for wait := 0; wait < 4 && s.HeightControl != 0; wait++ {
				tick(nil)
			}
			if s.Hover != 1 || s.HeightControl != 0 {
				t.Fatal("keyboard collective", s.Hover, s.HeightControl, s.Collective, c.Rendering.independentManeuvers, c.Rendering.Flight.Steps, c.Screen, c.fade != nil)
			}
			input.Axes[3] = 54
			for _, value := range []int16{0, 0, 24, 64, 90, 90, 90} {
				input.Axes[2] = value
				input.Lever = uint16(32768 + int(value)*200)
				tick(nil)
			}
			if s.Hover != 1 || s.Pose.Angles[1] == heading || s.Forward != 0 {
				t.Fatal("controller handover", s.Hover, s.Forward, s.Pose.Angles, heading)
			}
			endFilm(t, c)
			verifyFilm(t, c)
		})
	}
}
