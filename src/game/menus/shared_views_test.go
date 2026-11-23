package menus

import (
	"bytes"
	"encoding/json"
	a "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func displayFixture(t *testing.T) render3d.Scene {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(os.Getenv("GS2000_Q32_GROUND_SCENES"), "pad-scene.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s render3d.Scene
	if e = json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	return s
}

// Camera/scene-only substitutions keep rare material coverage repeatable. They
// are supplied render probes, not a claim of a naturally flown shared sortie.
func aimDisplay(p *RenderingPreview, s render3d.Scene, scale int, mode string) {
	p.Scene.World = s.World
	p.Scene.Camera = s.Camera
	p.Scene.RenderScale = scale
	p.Scene.PaletteBank = s.PaletteBank
	p.Scene.GraphicsOptions = s.GraphicsOptions
	if p.ViewControls != nil {
		p.ViewControls.Camera = s.Camera
	}
	p.presentation.mode = mode
	p.presentation.referenceDirty = true
	p.presentation.previous.Camera = s.Camera
	p.presentation.current.Camera = s.Camera
	p.presentation.alpha = .5
	p.Dirty = true
}
func checkDraw(t *testing.T, c *FrontEnd, p *RenderingPreview, name string) {
	t.Helper()
	if _, e := c.RenderRGBA(false); e != nil {
		t.Fatal(name, e)
	}
	before := p.FilmCheckpoint()
	var first []byte
	for n := 0; n < 3; n++ {
		p.Dirty = true
		im, e := c.RenderRGBA(false)
		if e != nil {
			t.Fatal(name, e)
		}
		if first == nil {
			first = bytes.Clone(im.Pix)
			if out := os.Getenv("GS2000_R7_VIEWS"); out != "" {
				os.MkdirAll(out, 0700)
				f, e := os.Create(filepath.Join(out, name+".png"))
				if e != nil {
					t.Fatal(e)
				}
				png.Encode(f, im)
				f.Close()
			}
		} else if !bytes.Equal(first, im.Pix) {
			t.Fatal(name, "redraw changed pixels")
		}
	}
	if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
		t.Fatal(name, "redraw changed authoritative checkpoint")
	}
	t.Log(name, "checkpoint and pixel purity")
}
func TestSharedViewsAndReplayPurity(t *testing.T) {
	s := displayFixture(t)
	c := mastProfileFixture(t, 4, "pass", "")
	defer c.AbandonFilmRecording()
	jFinishFade(t, c)
	if !c.Rendering.Initialized.Stores.VictimWingStores || !c.Rendering.r5Mechanics {
		t.Fatal("R6 capabilities lost")
	}
	for _, scale := range []int{1, 2} {
		for _, mode := range []string{"stepped", "smooth"} {
			for _, v := range []struct {
				name  string
				key   int
				shift bool
			}{{"cockpit", 1, false}, {"mast", 2, false}, {"external", 5, false}, {"tracked", 5, true}} {
				c.Handle(Actions{})
				if e := c.Handle(Actions{ViewKey: v.key, ViewShift: v.shift}); e != nil {
					t.Fatal(e)
				}
				p := c.Rendering
				aimDisplay(p, s, scale, mode)
				checkDraw(t, c, p, v.name+"-"+mode+string(rune('0'+scale)))
			}
		}
	}
	path := os.Getenv("GS2000_R7_FILM")
	if path == "" {
		t.Fatal("retained complete R6 film required")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	f, e := replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	filmAssets, err := a.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		cfg := configuration.Defaults()
		cfg.SimulationRenderScale = scale
		cfg.DisableEnemyAI = true
		cfg.DisableWingmenAI = true
		q, e := NewFilmPlayback(filmAssets, f, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for q.reachedSteps < 16 && !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
		p := q.Preview()
		host := &FrontEnd{Screen: ScreenReplay, Office: q.Engine.Office, Viewer: NewReplayViewer(q)}
		aimDisplay(p, s, scale, "stepped")
		host.Viewer.camera = s.Camera
		host.Viewer.previousCamera = s.Camera
		host.Viewer.cameraKnown = true
		checkDraw(t, host, p, "replay-"+string(rune('0'+scale)))
		host.Viewer.Demo = true
		aimDisplay(p, s, scale, "stepped")
		checkDraw(t, host, p, "demo-"+string(rune('0'+scale)))
		if e = q.Resume(); e != nil || q.Engine.Store != nil {
			t.Fatal("isolated Resume", e)
		}
		host.Viewer.Demo = false
		host.Viewer.Practice = true
		aimDisplay(p, s, scale, "smooth")
		checkDraw(t, host, p, "resume-"+string(rune('0'+scale)))
	}
}

func TestFullSceneMFDAndLateInkOwnership(t *testing.T) {
	c := naturalControllerProfile(t, true, "")
	defer c.AbandonFilmRecording()
	p := c.Rendering
	s := displayFixture(t)
	row := -1
	for _, e := range p.Initialized.Entities {
		if e.RuntimeRow > 5 && e.Participation != 0 && e.Kind&0x38 == 0 {
			for _, in := range p.instances {
				if in.ID == e.ID {
					row = e.RuntimeRow
					break
				}
			}
			if row >= 0 {
				break
			}
		}
	}
	if row < 0 {
		t.Fatal("source target absent")
	}
	p.Flight.Avionics.Target = uint16(row + 1)
	p.Flight.Avionics.TargetPanelZoom = 512
	// Supplied target-camera fixture; no shot, effect, result or flight advance.
	p.Initialized.Entities[row].Pose.H0 = s.Camera.H0
	p.Initialized.Entities[row].Pose.H1 = s.Camera.H1
	p.Initialized.Entities[row].Pose.Height = 0
	for _, scale := range []int{1, 2} {
		aimDisplay(p, s, scale, "stepped")
		p.Scene.GraphicsOptions |= 8
		p.Cockpit.PrimaryMFD = 5
		p.Cockpit.Arrangement = 0
		p.Cockpit.CameraMode = 0
		p.Cockpit.SystemFlags |= 1
		p.Cockpit.HUD = true
		p.Dirty = true
		checkDraw(t, c, p, "full-scene-mfd-"+string(rune('0'+scale)))
		if len(p.targetBrownMask) != len(p.Frame) {
			t.Fatal("full-scene material collector not reached")
		}
		p.Scene.GraphicsOptions &^= 8
		p.Dirty = true
		p.presentation.referenceDirty = true
		checkDraw(t, c, p, "model-only-mfd-"+string(rune('0'+scale)))
		for _, v := range p.targetBrownMask {
			if v != 0 {
				t.Fatal("model-only preview retained world material owner")
			}
		}
	}
	// A same-index opaque overlay still owns its exact destination. A conditional
	// destination remap leaves its real no-op destinations outside the exclusion.
	mask := bytes.Repeat([]byte{byte(render3d.SurfaceBrown)}, 64000)
	if e := erasePresentationWrites(mask, func(dst []byte) error {
		dst[100] = 0xb4
		if dst[101] >= 0x60 {
			dst[101] = 0x15
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if mask[100] != 0 || mask[102] != byte(render3d.SurfaceBrown) {
		t.Fatal("same-colour overlay ownership")
	}
	p.Inspection = false
	p.playerShake.rows = 3
	p.Frame = bytes.Repeat([]byte{0xb4}, 64000)
	p.skyMask = bytes.Repeat([]byte{byte(render3d.SurfaceBrown)}, 64000)
	p.Scene.RenderScale = 1
	if e := p.drawPlayerShake(); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(p.Frame, p.skyMask) {
		t.Fatal("display-start moved pixel and recipe differently")
	}
}
