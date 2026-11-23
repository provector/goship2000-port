//go:build !headless

package menus

import (
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestConnectedFidelityReview(t *testing.T) {
	out := os.Getenv("GS2000_L3_NATIVE_REVIEW")
	if out == "" {
		t.Skip("explicit disposable connected output required")
	}
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_L3_NATIVE_REVIEW_SCALE"))
	if err != nil || (scale != 1 && scale != 2) {
		t.Fatal("review scale must be1/2")
	}
	c := mastFixture(t, 6, "land")
	jFinishFade(t, c)
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	if mode := os.Getenv("GS2000_L3_NATIVE_REVIEW_PRESENTATION_MODE"); mode != "" {
		options.PresentationMode = mode
	}
	if err = options.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Runtime = options
	p := c.Rendering
	p.Scene.RenderScale = scale
	p.Frame, p.worldFrame = make([]byte, p.Scene.TargetPixels()), make([]byte, p.Scene.TargetPixels())
	p.presentation.mode = options.PresentationMode
	p.resetPresentation()
	revision, mission := c.Store.State().Revision, c.Mission.Data.ObjectHash
	if err = writeDevelopmentCheckpoint(out, 0, c); err != nil {
		t.Fatal(err)
	}
	if err = RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 1200, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if err = writeDevelopmentCheckpoint(out, 1200, c); err != nil {
		t.Fatal(err)
	}
	if c.Store.State().Revision != revision || c.Mission.Data.ObjectHash != mission {
		t.Fatal("native view/GPS input changed saves or regenerated mission")
	}
	if p.GPS != nil || c.sceneConfirmationPresented() || c.Screen != ScreenScene {
		t.Fatal("native review left a modal or scene")
	}
}
