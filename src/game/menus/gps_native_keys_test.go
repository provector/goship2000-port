package menus

import (
	"gunship2000-assets/game/navigation"
	"testing"
)

func nativeM(t *testing.T, c *FrontEnd) {
	t.Helper()
	var device windowInput
	a := device.Read(Buttons{GPSKey: sampleGPSCommandKey(func(k int32) bool { return k == 'M' })})
	editorAction(t, c, a)
	if c.Rendering.GPS == nil || c.Rendering.GPS.Support == nil || !c.Rendering.GPS.Support.Editing {
		t.Fatal("native M sampler/latch/FrontEnd/GPS admission did not reach selection")
	}
	editorAction(t, c, device.Read(Buttons{}))
	for _, wantLow := range []bool{true, false} {
		space := sampleGPSCommandKey(func(k int32) bool { return k == ' ' })
		editorAction(t, c, device.Read(Buttons{GPSKey: space, Space: true}))
		if c.Rendering.GPS == nil || c.Rendering.GPS.Low != wantLow || !c.Rendering.GPS.Support.Editing || c.Rendering.Initialized.Support.Request != 0 {
			t.Fatal("physical Space became selector/Support command instead of zoom", wantLow)
		}
		editorAction(t, c, device.Read(Buttons{}))
	}
	before := filmJSON(c.Rendering.Flight.Navigation)
	selected := c.Rendering.GPS.Selected
	for _, raw := range []int32{'F', 'N', 'B'} {
		editorAction(t, c, device.Read(Buttons{GPSKey: sampleGPSCommandKey(func(k int32) bool { return k == raw })}))
		editorAction(t, c, device.Read(Buttons{}))
		if c.Rendering.GPS.Editing || c.Rendering.GPS.Selected != selected || string(before) != string(filmJSON(c.Rendering.Flight.Navigation)) {
			t.Fatal("Support mode leaked into route/member/home command", raw)
		}
	}
}

func TestNativeMissionAssetsBothArtilleryLifecycles(t *testing.T) {
	for _, n := range []int{0, 1} {
		t.Run(map[int]string{0: "first-source-battery", 1: "second-source-battery"}[n], func(t *testing.T) { runAuthoredArtilleryGPSRecording(t, n, true) })
	}
}

// The prior Campaign history is a supplied base approach, clearly separate from
// the freshly generated Air mission. No target hit, command or outcome is staged.
func TestGeneratedAirNativeMissionAssetsLifecycle(t *testing.T) {
	c := developmentScenarioController(t, "campaign-return")
	campaignNaturalBaseEnd(t, c)
	loopAcknowledge(t, c)
	seed := int64(2)
	c.MissionSeed = &seed
	readyMission(t, c)
	if c.Mission.Data.Support == nil || c.Mission.Data.Support.Mode != 2 {
		t.Fatal("source generated Air option absent")
	}
	startRendering(t, c)
	jFinishFade(t, c)
	runAirSupportAndRecord(t, c, "p6-generated", true)
}

func TestNativeGPSMKeyReachesModalOnce(t *testing.T) {
	var device windowInput
	var routes [5]navigation.Route
	g := navigation.New(&routes, [5]navigation.Point{{H0: 0x40000, H1: 0x40000}}, [5]bool{true})
	g.EnableSupport(0x31, 0, 0)
	pressed := func(key int32) bool { return key == 'M' }
	key := sampleGPSCommandKey(pressed)
	if key != "m" {
		t.Fatal("native physical key producer omitted Mission Assets", key)
	}
	a := device.Read(Buttons{GPSKey: key})
	if _, err := g.Handle(navigation.Input{Key: a.GPSKey}); err != nil || !g.Support.Editing {
		t.Fatal("physical make did not reach actual GPS consumer", err, g.Support)
	}
	// The make event is consumed once; holding M neither retriggers nor leaks a
	// flight chaff command through this modal-specific producer.
	a = device.Read(Buttons{GPSKey: key})
	if a.GPSKey != "" {
		t.Fatal("held GPS make repeated", a.GPSKey)
	}
	if _, err := g.Handle(navigation.Input{Key: a.GPSKey}); err != nil || !g.Support.Editing {
		t.Fatal(err, g.Support)
	}
	device.Read(Buttons{})
	a = device.Read(Buttons{GPSKey: sampleGPSCommandKey(pressed)})
	g.Handle(navigation.Input{Key: a.GPSKey})
	if g.Support.Editing {
		t.Fatal("released/repressed M did not leave support selection")
	}
	for _, raw := range []int32{'F', ' ', '1', '2', '3', 'B', 'N', 'C', '6', '7', 'M'} {
		got := sampleGPSCommandKey(func(k int32) bool { return k == raw })
		if got == "" {
			t.Fatalf("existing physical GPS key %q lost", raw)
		}
	}
	if got := sampleGPSCommandKey(func(k int32) bool { return k == 'Q' }); got != "" {
		t.Fatal("unrelated key acquired GPS command", got)
	}
}
