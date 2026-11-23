package menus

import (
	"bytes"
	"reflect"
	"testing"

	"gunship2000-assets/game/cockpit"
)

func beginInitialization(t *testing.T, c *FrontEnd) {
	t.Helper()
	missionClick(t, c, 3)
	missionClick(t, c, 9)
	missionClick(t, c, 9)
	for i := 0; i < 200 && c.Screen == ScreenSettings; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenSceneInitialization {
		t.Fatalf("APU did not enter initialization:%s %s", c.Screen, c.Message)
	}
}

func finishInitialization(t *testing.T, c *FrontEnd) {
	t.Helper()
	for i := 0; i < 8 && c.Screen == ScreenSceneInitialization; i++ {
		c.MarkScenePresented()
		if err := c.AdvanceSceneInitialization(SceneInitializationLineSeconds); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenScene || c.SceneEntry != nil {
		t.Fatal("initialization did not publish scene")
	}
}

func TestInitializationNeedsPresentationAndKeepsSession(t *testing.T) {
	c := renderingController(t)
	data := c.Mission.Data
	before := c.Store.State()
	beginInitialization(t, c)
	opening := c.Frame(false)
	for i := 0; i < 4; i++ {
		if err := c.AdvanceSceneInitialization(1); err != nil {
			t.Fatal(err)
		}
		c.Frame(false)
	}
	if c.SceneEntry.Stage != 0 || c.Rendering != nil {
		t.Fatal("unpresented stage advanced/prepared")
	}
	c.MarkScenePresented()
	if err := c.AdvanceSceneInitialization(SceneInitializationLineSeconds); err != nil {
		t.Fatal(err)
	}
	if c.SceneEntry.Stage != 1 || c.Rendering == nil || bytes.Equal(opening, c.Frame(false)) {
		t.Fatal("native candidate/readiness stage missing")
	}
	stage := *c.SceneEntry
	act(t, c, Actions{Quit: true})
	if err := c.AdvanceSceneInitialization(1); err != nil {
		t.Fatal(err)
	}
	if *c.SceneEntry != stage {
		t.Fatal("initialization advanced behind quit")
	}
	act(t, c, Actions{Cancel: true})
	if c.Screen != ScreenSceneInitialization || *c.SceneEntry != stage {
		t.Fatal("quit cancel reset initialization")
	}
	act(t, c, Actions{Selector1: true, Selector2: true, Cancel: true, EndMission: true})
	if c.Screen != ScreenSceneInitialization {
		t.Fatal("input crossed initialization")
	}
	finishInitialization(t, c)
	if c.Mission.Data != data || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("initialization regenerated mission or changed progression")
	}
	if b, e := c.Store.RecoveryCandidate(); e != nil || b != nil {
		t.Fatal("initialization created recovery", e)
	}
}

func TestInitializationFailureKeepsEditablePreparation(t *testing.T) {
	c := renderingController(t)
	s := c.Mission
	before := c.Store.State()
	beginInitialization(t, c)
	c.Office.Assets.World = nil
	c.MarkScenePresented()
	err := c.AdvanceSceneInitialization(SceneInitializationLineSeconds)
	if err == nil || c.Screen != ScreenSettings || c.Mission != s || c.Rendering != nil || c.SceneEntry != nil || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("failed native preparation published/lost state", err)
	}
}

func TestSceneConfirmationsUseSourceIndicesAndConsumeEdges(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	session := c.Mission
	state := c.Store.State()
	p := c.Rendering
	camera, phase := p.Scene.Camera, p.Visual.Phase
	original := c.Frame(false)
	w := windowInput{}
	view := officeViewport(960, 720, false)
	b := Buttons{Inside: true, AltQ: true}
	act(t, c, w.ReadViewport(b, view))
	if c.Screen != ScreenQuit || c.sceneEndConfirmation {
		t.Fatal("AltQ scene confirmation missing")
	}
	box, e := cockpit.SceneConfirmationBounds(c.Office.Assets.Cockpit, false)
	if e != nil {
		t.Fatal(e)
	}
	f := c.Frame(false)
	at := box.Top*320 + box.Left
	a := c.Office.Assets.Cockpit
	if f[at] != a.ScenePresentation.ConfirmationRemap[original[at]]+a.ScenePresentation.Quit.Background {
		t.Fatal("scene background is not source destination remap")
	}
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			if x < box.Left || x > box.Right || y < box.Top || y > box.Bottom {
				if f[y*320+x] != original[y*320+x] {
					t.Fatal("scene prompt changed outside source rectangle")
				}
			}
		}
	}
	act(t, c, w.ReadViewport(b, view))
	if c.Screen != ScreenQuit {
		t.Fatal("held AltQ dismissed modal")
	}
	b = Buttons{Inside: true, Text: []rune{'N'}}
	act(t, c, w.ReadViewport(b, view))
	if c.Screen != ScreenScene || c.Rendering != p || p.Scene.Camera != camera || p.Visual.Phase != phase {
		t.Fatal("quit cancellation changed scene")
	}
	for _, cancel := range []Actions{{Cancel: true}, {Text: []rune{'n'}}} {
		act(t, c, Actions{EndMission: true})
		if !c.sceneEndConfirmation || c.Screen != ScreenQuit {
			t.Fatal("AltE modal missing")
		}
		act(t, c, cancel)
		if c.Screen != ScreenScene || p.Scene.Camera != camera || p.Visual.Phase != phase {
			t.Fatal("end cancellation changed scene")
		}
	}
	b = Buttons{Inside: true, AltE: true}
	act(t, c, w.ReadViewport(b, view))
	b.ConfirmQuit = true
	act(t, c, w.ReadViewport(b, view))
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || c.Mission != session || c.Mission.Data != session.Data || !reflect.DeepEqual(state, c.Store.State()) {
		t.Fatal("end confirmation created outcome/lost session")
	}
	act(t, c, w.ReadViewport(b, view))
	if c.Screen != ScreenBriefing {
		t.Fatal("held confirmation leaked into briefing")
	}
	if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("ending preview created backup", err)
	}
	startRendering(t, c)
	act(t, c, Actions{Quit: true})
	act(t, c, Actions{ConfirmQuit: true})
	if !c.ExitRequested {
		t.Fatal("direct physical Y edge quit regressed")
	}
}

func TestSceneModalPreservesFadeAndEndClearsIt(t *testing.T) {
	c := renderingController(t)
	c.FadeMilliseconds = 150
	beginInitialization(t, c)
	finishInitialization(t, c)
	if c.fade == nil {
		t.Fatal("initialization did not use shared fade")
	}
	tick := c.fade.Tick
	act(t, c, Actions{Quit: true})
	if _, err := c.RenderRGBA(false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	act(t, c, Actions{Cancel: true})
	if c.fade == nil || c.fade.Tick != tick {
		t.Fatal("quit cancellation changed fade")
	}
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || c.fade != nil {
		t.Fatal("end return retained scene transition")
	}
}

func TestSceneMusicAbsenceIncludesInitializationReenableAndReentry(t *testing.T) {
	p, _ := audioTestPlayer(t)
	p.SetDeviceClock(true)
	c := renderingController(t)
	syncFrontEndAudio(p, c)
	data := make([]float32, 8)
	p.Render(data, 8)
	beginInitialization(t, c)
	syncFrontEndAudio(p, c)
	checkSilent := func() {
		t.Helper()
		p.Render(data, 8)
		for _, x := range data {
			if x != 0 {
				t.Fatal("background music remained in scene", p.Status())
			}
		}
		if p.Status().MusicOwner != "silent" || !p.Status().EffectsEnabled {
			t.Fatal("silent owner/global effects state", p.Status())
		}
	}
	checkSilent()
	p.SetMusicEnabled(false)
	p.SetMusicEnabled(true)
	syncFrontEndAudio(p, c)
	checkSilent()
	finishInitialization(t, c)
	for i := 0; i < 3; i++ {
		c.Frame(false)
		syncFrontEndAudio(p, c)
		checkSilent()
	}
	act(t, c, Actions{Quit: true})
	syncFrontEndAudio(p, c)
	checkSilent()
	act(t, c, Actions{Text: []rune{'n'}})
	syncFrontEndAudio(p, c)
	checkSilent()
	// Synthetic effect verifies category independence; no new inflight trigger.
	p.PlayCue(0x22)
	syncFrontEndAudio(p, c)
	p.Render(data[:1], 1)
	if data[0] != 100.0/32768 {
		t.Fatal("scene music policy disabled effects")
	}
	p.Render(data, 8)
	checkSilent()
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	syncFrontEndAudio(p, c)
	p.Render(data[:1], 1)
	if data[0] != 1000.0/32768 {
		t.Fatal("briefing return did not restore menu owner")
	}
	startRendering(t, c)
	syncFrontEndAudio(p, c)
	p.SetMusicEnabled(false)
	p.SetMusicEnabled(true)
	checkSilent()
}
