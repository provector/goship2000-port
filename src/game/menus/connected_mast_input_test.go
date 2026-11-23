//go:build !headless

package menus

import (
	"fmt"
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Opt-in production-input wrapper with an actual accepted Longbow Training
// mission. External XTest events enter raylib normally; no target, camera pose
// or contact row is supplied/overridden by this test.
func TestConnectedMastInput(t *testing.T) {
	out := os.Getenv("GS2000_L2_NATIVE_MAST")
	if out == "" {
		t.Skip("explicit disposable connected output required")
	}
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	scale := 1
	if raw := os.Getenv("GS2000_L2_NATIVE_MAST_SCALE"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || (v != 1 && v != 2) {
			t.Fatal("GS2000_L2_NATIVE_MAST_SCALE must be1 or2")
		}
		scale = v
	}
	c := mastFixture(t, 1, "land")
	jFinishFade(t, c)
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	if mode := os.Getenv("GS2000_L2_NATIVE_MAST_PRESENTATION_MODE"); mode != "" {
		options.PresentationMode = mode
	}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Runtime = options
	p := c.Rendering
	p.Scene.RenderScale = scale
	p.Frame = make([]byte, p.Scene.TargetPixels())
	p.worldFrame = make([]byte, p.Scene.TargetPixels())
	p.presentation.mode = options.PresentationMode
	p.resetPresentation()
	before := c.Store.State().Revision
	missionHash := c.Mission.Data.ObjectHash
	if err := writeDevelopmentCheckpoint(out, 0, c); err != nil {
		t.Fatal(err)
	}
	if err := RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 1000, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if err := writeDevelopmentCheckpoint(out, 1000, c); err != nil {
		t.Fatal(err)
	}
	if c.Store.State().Revision != before || c.Mission.Data.ObjectHash != missionHash {
		t.Fatal("physical view/target input mutated persistent state or regenerated mission")
	}
	if p.ViewControls.Mode != 7 || p.Cockpit.CameraMode != 7 || p.Scene.Scale() != scale {
		t.Fatalf("physical F2/reentry not retained: mode%d camera%d scale%d", p.ViewControls.Mode, p.Cockpit.CameraMode, p.Scene.Scale())
	}
	if p.Flight.Avionics.Target == 0 {
		t.Fatal("physical Return did not acquire a naturally available contact; no target override permitted")
	}
	if p.GPS != nil || c.sceneConfirmationPresented() {
		t.Fatal("probe left a modal open")
	}
	fmt.Printf("L2 native Longbow mast: scale%d target%d retained; revision%d unchanged; mission%s\n", scale, p.Flight.Avionics.Target, before, missionHash)
}
