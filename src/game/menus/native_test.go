//go:build !headless

package menus

import (
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	"os"
	"strconv"
	"testing"
)

func TestNativeLoop(t *testing.T) {
	out := os.Getenv("GS2000_R81_NATIVE")
	if out == "" {
		t.Skip("explicit bounded owned window")
	}
	assets, _ := benchmarkScenes(t)
	raw, e := os.ReadFile(os.Getenv("GS2000_R7_FILM"))
	if e != nil {
		t.Fatal(e)
	}
	f, e := replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	cfg := configuration.Defaults()
	scale, _ := strconv.Atoi(os.Getenv("GS2000_R81_SCALE"))
	if scale != 1 && scale != 2 {
		scale = 2
	}
	cfg.SimulationRenderScale = scale
	q, e := NewFilmPlayback(assets, f, cfg)
	if e != nil {
		t.Fatal(e)
	}
	for q.reachedSteps < 16 && !q.Done {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if e = q.Resume(); e != nil {
		t.Fatal(e)
	}
	c := q.Engine
	// Playback's logical host deliberately has no office raster. The native
	// window's initial upload also visits Office.Frame before the scene frame.
	c.Office, e = NewOffice(assets, c.Office.State, 1)
	if e != nil {
		t.Fatal(e)
	}
	p := q.Preview()
	p.Flight.Paused = false
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	options.MouseCapture = "disabled"
	options.PresentationMode = "smooth"
	options.PresentationFPS = 60
	if os.Getenv("GS2000_R81_MODE") == "stepped" {
		options.PresentationMode = "stepped"
	}
	p.presentation.mode = options.PresentationMode
	if e = c.Handle(Actions{CompressionReset: true}); e != nil {
		t.Fatal(e)
	}
	t.Log("normal resumed population", p.Scene.World, len(p.Scene.Instances), "camera", p.Cockpit.CameraMode, "MFD", p.Cockpit.PrimaryMFD, "mode", p.presentation.mode, "compression", p.Flight.Compression.Rate())
	no := false
	if e = RunOfficeWindow(c.Office, WindowOptions{FrontEnd: c, Runtime: options, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 720, CaptureDirectory: out, MusicEnabled: &no, SoundEffectsEnabled: &no}); e != nil {
		t.Fatal(e)
	}
}
