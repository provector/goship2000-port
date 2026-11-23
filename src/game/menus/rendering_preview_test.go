package menus

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
)

var renderingAssetsOnce sync.Once
var renderingAssets *assetdata.OfficeAssets
var renderingAssetsErr error

func renderingController(t *testing.T) *FrontEnd {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("prepared StepH integration is opt-in")
	}
	renderingAssetsOnce.Do(func() { renderingAssets, renderingAssetsErr = assetdata.LoadFrontEndAssets(dir) })
	if renderingAssetsErr != nil {
		t.Fatal(renderingAssetsErr)
	}
	c := missionController(t)
	// This retained pre-R2 physics/render/replay fixture keeps its recorded
	// R12 no-hostile interpretation. R21 acceptance constructs a new profile.
	c.simulationProfile = replay.R12Compatibility
	c.Office.Assets.World = renderingAssets.World
	c.Office.Assets.Cockpit = renderingAssets.Cockpit
	c.Office.Assets.Replay = renderingAssets.Replay // O.1 recording requires actual Preparation25 text readiness.
	readyMission(t, c)
	return c
}

func startRendering(t *testing.T, c *FrontEnd) {
	t.Helper()
	missionClick(t, c, 3)
	missionClick(t, c, 9)
	missionClick(t, c, 9)
	for i := 0; i < 300 && c.Screen != ScreenScene; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
		c.MarkScenePresented()
		if err := c.AdvanceSceneInitialization(1.0 / 60); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenScene {
		t.Fatalf("accepted preflight stayed on %s: %s", c.Screen, c.Message)
	}
}

func TestRenderingActualPreflightHandoffReturnAndQuit(t *testing.T) {
	c := renderingController(t)
	session := c.Mission
	before := c.Store.State()
	draws := append([]byte(nil), session.Data.Objects[0].Raw[:]...)
	startRendering(t, c)
	p := c.Rendering
	if p.Scene.World != session.Data.SourceWorld.TerrainName || p.MissionObjectHash != session.Data.ObjectHash || len(p.Scene.Instances) == 0 {
		t.Fatal("scene replaced mission inputs")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("preview manufactured progression")
	}
	if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("rendering created recovery", err)
	}
	first := c.Frame(false)
	phase := p.Visual.Phase
	for i := 0; i < 5; i++ {
		if !bytes.Equal(first, c.Frame(false)) {
			t.Fatal("draw changed phase or geometry")
		}
	}
	if phase != p.Visual.Phase || !bytes.Equal(draws, session.Data.Objects[0].Raw[:]) {
		t.Fatal("draw mutated source state")
	}
	if bytes.Equal(p.worldFrame, p.Frame) {
		t.Fatal("cockpit composition not exercised")
	}
	p.Cockpit.CameraMode = 1
	p.Cockpit.HUD = false
	p.Dirty = true
	if !bytes.Equal(c.Frame(false), p.worldFrame) {
		t.Fatal("cockpit leaked into retained world destination")
	}
	act(t, c, Actions{Quit: true})
	if err := c.AdvanceRendering(1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RenderRGBA(false); err != nil {
		t.Fatal("mixed palette quit", err)
	}
	act(t, c, Actions{Text: []rune{'N'}})
	if c.Screen != ScreenScene || c.Rendering != p || p.Visual.Phase != phase {
		t.Fatal("quit cancellation changed scene")
	}
	act(t, c, Actions{Cancel: true})
	if c.Screen != ScreenScene {
		t.Fatal("Escape intercepted normal flight")
	}
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing || c.Mission != session || c.Mission.Cockpit.Ready {
		t.Fatal("return lost mission/reset boundary")
	}
	startRendering(t, c)
	act(t, c, Actions{Selector2: true})
	if c.Screen != ScreenScene || c.Mission != session {
		t.Fatal("selector intercepted normal flight")
	}
	act(t, c, Actions{EndMission: true})
	act(t, c, Actions{ConfirmQuit: true})
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("return created outcome")
	}
}

func TestRenderingClockPauseAndInspectionOwnership(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	p := c.Rendering
	p.Inspection = true // This test isolates the visual owner; TestJ covers live flight partitioning.
	mission := c.Mission.Data
	original := mission.ObjectHash
	for i := 0; i < 60; i++ {
		if err := c.AdvanceRendering(1.0 / 60); err != nil {
			t.Fatal(err)
		}
	}
	if p.Visual.Phase != 12 {
		t.Fatal("render cadence changed visual rate", p.Visual.Phase)
	}
	at60 := c.Frame(false)
	p.Visual = render3d.VisualState{}
	p.clock = tickAccumulator{Rate: p.Rate}
	clear(p.worldFrame)
	p.Dirty = true
	for i := 0; i < 144; i++ {
		if err := c.AdvanceRendering(1.0 / 144); err != nil {
			t.Fatal(err)
		}
	}
	if p.Visual.Phase != 12 || !bytes.Equal(at60, c.Frame(false)) {
		t.Fatal("144FPS changed logical visual result")
	}
	p.Paused = true
	for i := 0; i < 144; i++ {
		if err := c.AdvanceRendering(1.0 / 144); err != nil {
			t.Fatal(err)
		}
	}
	if p.Visual.Phase != 12 {
		t.Fatal("pause advanced visual state")
	}
	if err := c.StepRendering(1); err != nil {
		t.Fatal(err)
	}
	if p.Visual.Phase != 13 {
		t.Fatal("single step")
	}
	old := p.Scene.Camera
	c.MoveInspectionCamera(.1, 1, 0, 0, 0, 0)
	if old == p.Scene.Camera || mission.ObjectHash != original || c.Mission.Data != mission {
		t.Fatal("inspection navigation mutated mission")
	}
	if err := c.ConfigureRenderingInspection(RenderingInspection{World: "GULF5", Arrangement: 2, Rate: 12, Paused: true, Exterior: true}); err != nil {
		t.Fatal(err)
	}
	if len(p.Scene.Instances) != 0 || !strings.Contains(p.DiagnosticPolicy, "Standalone development world override") {
		t.Fatal("unrelated world retained mission units")
	}
	if err := c.ConfigureRenderingInspection(RenderingInspection{Rate: 0}); err == nil {
		t.Fatal("invalid preview cadence")
	}
}

func TestRenderingUnsupportedCandidateDoesNotPublish(t *testing.T) {
	c := renderingController(t)
	before := c.Store.State()
	final, err := finalPreflight(c.Mission)
	if err != nil {
		t.Fatal(err)
	}
	old := c.Office.Assets.World
	c.Office.Assets.World = nil
	if err = c.enterRenderingPreview(final); err == nil {
		t.Fatal("accepted missing assets")
	}
	c.Office.Assets.World = old
	if c.Screen != ScreenBriefing || c.Rendering != nil || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("failed visual preparation published/saved")
	}
	bad := render3d.Camera{Pose: render3d.Pose{H0: 0xffffffff, H1: 1, Height: 512}}
	if err = c.enterRenderingPreview(final); err != nil {
		t.Fatal(err)
	}
	if err = c.ConfigureRenderingInspection(RenderingInspection{Camera: &bad, Rate: 12}); err == nil {
		t.Fatal("unsupported coordinate accepted")
	}
}

func TestRenderingAudioPolicyStopsMenuOwnersOnly(t *testing.T) {
	p, c := audioTestPlayer(t)
	syncFrontEndAudio(p, c)
	p.PlayCue(0x22)
	c.Screen = ScreenScene
	syncFrontEndAudio(p, c)
	data := make([]float32, 4)
	p.Render(data, 4)
	for _, v := range data {
		if v != 0 {
			t.Fatal("menu sound leaked into preview")
		}
	}
	if p.Status().MusicOwner != "silent" {
		t.Fatal("preview audio owner")
	}
	c.previousScreen = ScreenScene
	c.Screen = ScreenQuit
	syncFrontEndAudio(p, c)
	c.Screen = ScreenScene
	syncFrontEndAudio(p, c)
	if p.Status().MusicOwner != "silent" {
		t.Fatal("quit cancelled into office music")
	}
	c.Screen = ScreenBriefing
	syncFrontEndAudio(p, c)
	p.Render(data[:1], 1)
	if data[0] != 1000.0/32768 {
		t.Fatal("return did not restore menu owner", data[0])
	}
}

func TestRenderingAcceptanceKeepsPreferenceSaveBoundary(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "failure"}[fail], func(t *testing.T) {
			c := renderingController(t)
			before := c.Store.State()
			c.Mission.Settings.EnemyQuality = 1
			c.Mission.SettingsChanged = true
			final, err := finalPreflight(c.Mission)
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				if err = os.Mkdir(filepath.Join(c.Store.Directory, ".pending-commit.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err = c.enterRenderingPreview(final)
			if fail {
				if err == nil || c.Rendering != nil || c.Mission != nil || c.Screen != ScreenOffice || !reflect.DeepEqual(before, c.Store.State()) {
					t.Fatal("failed preference save published a scene")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			p := c.Store.State().SelectedPilot()
			if p.Preflight == nil || p.Preflight.EnemyQuality != 1 {
				t.Fatal("accepted settings not copied back")
			}
			if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
				t.Fatal("settings acceptance made gameplay backup", err)
			}
		})
	}
}

func TestRenderingHandoffConsumesHeldSelectorThroughWindowWrapper(t *testing.T) {
	c := renderingController(t)
	missionClick(t, c, 3)
	x, y, ok := missionActionPoint(c, 9)
	if !ok {
		t.Fatal("APU hotspot")
	}
	w := windowInput{}
	viewport := officeViewport(960, 720, false)
	buttons := Buttons{X: x, Y: y, Inside: true, LeftMouse: true}
	act(t, c, w.ReadViewport(buttons, viewport))
	buttons.LeftMouse = false
	act(t, c, w.ReadViewport(buttons, viewport))
	// APU is Selector1-only. Press both buttons on its final edge, then keep
	// holding them across the scene change: Selector2 must not fire again.
	buttons.LeftMouse = true
	buttons.RightMouse = true
	act(t, c, w.ReadViewport(buttons, viewport))
	for i := 0; i < 300 && c.Screen != ScreenScene; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
		c.MarkScenePresented()
		if err := c.AdvanceSceneInitialization(1.0 / 60); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenScene {
		t.Fatalf("APU wrapper did not enter scene:%s %s", c.Screen, c.Message)
	}
	act(t, c, w.ReadViewport(buttons, viewport))
	if c.Screen != ScreenScene {
		t.Fatal("held selector carried through into office return")
	}
	buttons.RightMouse = false
	act(t, c, w.ReadViewport(buttons, viewport))
	buttons.RightMouse = true
	act(t, c, w.ReadViewport(buttons, viewport))
	if c.Screen != ScreenScene {
		t.Fatal("new selector press intercepted normal flight")
	}
}
