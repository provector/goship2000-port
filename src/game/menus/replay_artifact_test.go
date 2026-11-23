package menus

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"gunship2000-assets/game/world"
)

// Private film input is opt-in; neither its identifier nor a personal save is
// compiled into the port. The focused R14 runner requires this fixture.
func TestIncidentReplayPixelsAndAuthority(t *testing.T) {
	path := os.Getenv("GS2000_R14_INCIDENT_FILM")
	if path == "" {
		t.Skip("set GS2000_R14_INCIDENT_FILM for the retained incident")
	}
	c := renderingController(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	film, err := replay.Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	open := func() {
		t.Helper()
		q, e := NewFilmPlayback(c.Office.Assets, film, configuration.Defaults())
		if e != nil {
			t.Fatal(e)
		}
		c.Viewer = NewReplayViewer(q)
		c.Screen = ScreenReplay
	}
	advance := func(counter int) {
		t.Helper()
		for frames := 0; int(c.Viewer.Playback.Preview().Cockpit.MissionCounter) < counter; frames++ {
			if frames > 10000 {
				t.Fatal("counter not reached")
			}
			if e := c.AdvanceReplay(1.0 / 60); e != nil {
				t.Fatal(e)
			}
		}
	}
	capture := func(name string, pixels []byte) {
		t.Helper()
		out := os.Getenv("GS2000_R14_CAPTURE")
		if out == "" {
			return
		}
		if e := os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
		img, e := indexedFrameRGBA(pixels, c.replayPalette())
		if e != nil {
			t.Fatal(e)
		}
		fp, e := os.Create(filepath.Join(out, name+".png"))
		if e != nil {
			t.Fatal(e)
		}
		e = png.Encode(fp, img)
		fp.Close()
		if e != nil {
			t.Fatal(e)
		}
	}
	// Disable support metadata in a private renderer copy, not source geometry or
	// recorded state. Height regions are not a raster input. This counterfactual
	// therefore isolates observer admission while retaining both rotor paths.
	legacyFrame := func() []byte {
		t.Helper()
		p := c.Viewer.Playback.Preview()
		original := p.Renderer
		renderer := *original
		assets := *original.Assets
		assets.Geometries = map[string]*world.Geometry{}
		for k, g := range original.Assets.Geometries {
			copy := *g
			copy.Heights = nil
			assets.Geometries[k] = &copy
		}
		renderer.Assets = &assets
		p.Renderer = &renderer
		defer func() { p.Renderer = original }()
		return append([]byte(nil), c.replayFrame(false)...)
	}
	records := []map[string]any{}
	for _, site := range []struct {
		name    string
		counter int
		yaw     uint16
	}{{"position02", 12, 0xc000}, {"position13", 78, 0xc780}} {
		open()
		advance(site.counter)
		v := c.Viewer
		p := v.Playback.Preview()
		v.yaw = site.yaw
		for i := 0; i < 40; i++ {
			v.updateCamera()
		}
		v.alpha = 1
		summary, ok := v.Playback.Summary()
		if !ok || summary.Score != 630 || summary.TotalKills != 19 || summary.Elapsed/6 != 22*60+28 || !summary.Primary {
			t.Fatal("wrong incident summary", summary)
		}
		if v.Actor != 0 || p.Flight.State.Pose.Height != 2 {
			t.Fatal("wrong tracked actor/ground pose")
		}
		baseScale := p.Scene.RenderScale
		for _, scale := range []int{1, 2} {
			p.Scene.RenderScale = scale
			checkpoint := p.FilmCheckpoint()
			sourceCamera := v.camera
			before := legacyFrame()
			after := append([]byte(nil), c.replayFrame(false)...)
			w := 320 * scale
			at := 20*scale*w + 40*scale
			if before[at] != 105 || after[at] == 105 {
				t.Fatal(site.name, scale, "roof writer not distinguished", before[at], after[at])
			}
			if bytes.Equal(before, after) {
				t.Fatal("observer repair did not change pixels")
			}
			if !reflect.DeepEqual(checkpoint, p.FilmCheckpoint()) || v.camera != sourceCamera {
				t.Fatal("display changed source authority")
			}
			// A pause/ordinary redraw is deterministic; incoming viewport contents are
			// overwritten. It cannot produce or retain this face without current draws.
			again := append([]byte(nil), c.replayFrame(false)...)
			if !bytes.Equal(after, again) {
				t.Fatal("paused redraw changed pixels")
			}
			for y := 3 * scale; y <= 163*scale; y++ {
				for x := 2 * scale; x <= 225*scale; x++ {
					v.frame[y*w+x] = 254
				}
			}
			fresh := c.replayFrame(false)
			for y := 3 * scale; y <= 163*scale; y++ {
				if !bytes.Equal(fresh[y*w+2*scale:y*w+225*scale+1], after[y*w+2*scale:y*w+225*scale+1]) {
					t.Fatal("viewport retains old pixels")
				}
			}
			tag := site.name + "-" + string(rune('0'+scale)) + "x"
			capture(tag+"-before", before)
			capture(tag+"-after", after)
			records = append(records, map[string]any{"position": site.name, "counter": p.Cockpit.MissionCounter, "cursor": v.Playback.Cursor, "step": p.Flight.Steps, "scale": scale, "source_camera": sourceCamera, "pixel": []int{40 * scale, 20 * scale}, "before": before[at], "after": after[at], "canonical_checkpoint_unchanged": true})
		}
		p.Scene.RenderScale = baseScale
	}
	// Rewind traverses ordinary playback from the same recorded beginning; a
	// viewer operation cannot smuggle a source camera or result change into it.
	open()
	advance(12)
	v := c.Viewer
	v.alpha = 1
	for i := 0; i < 40; i++ {
		v.updateCamera()
	}
	first := append([]byte(nil), c.replayFrame(false)...)
	if err = c.activateReplayControl(0); err != nil {
		t.Fatal(err)
	}
	advance(12)
	v = c.Viewer
	v.alpha = 1
	for i := 0; i < 40; i++ {
		v.updateCamera()
	}
	if !bytes.Equal(first, c.replayFrame(false)) {
		t.Fatal("rewind changed matched position")
	}
	// Initial adjacent clear frame: no support-volume correction is admitted.
	open()
	p := c.Viewer.Playback.Preview()
	scene := p.Scene
	scene.Camera = c.Viewer.camera
	scene.Instances = p.instances
	var focus render3d.Instance
	for _, in := range scene.Instances {
		if in.ID == p.Initialized.PlayerID {
			focus = in
		}
	}
	if got := p.Renderer.ObserverCamera(scene, focus); got != scene.Camera {
		t.Fatal("nearby clear camera changed", got, scene.Camera)
	}
	if !bytes.Equal(legacyFrame(), c.replayFrame(false)) {
		t.Fatal("clear frame changed beyond shared rotor presentation")
	}
	capture("adjacent-clear", c.replayFrame(false))
	// Different tracked aircraft and legitimate close helicopter view share the
	// same correction. Helicopter meshes have no support regions: no culling hack.
	if err = c.activateReplayControl(10); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		c.Viewer.updateCamera()
	}
	c.Viewer.alpha = 1
	capture("different-tracked-aircraft", c.replayFrame(false))
	if out := os.Getenv("GS2000_R14_CAPTURE"); out != "" {
		b, _ := json.MarshalIndent(records, "", "  ")
		if e := os.WriteFile(filepath.Join(out, "positions.json"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}

func TestLegitimateCarrierAndAircraftGeometryRemain(t *testing.T) {
	c := mastFixture(t, 6, "carrier")
	jFinishFade(t, c)
	p := c.Rendering
	var focus render3d.Instance
	for _, in := range p.instances {
		if in.ID == p.Initialized.PlayerID {
			focus = in
		}
	}
	if focus.ID == "" {
		t.Fatal("source carrier flight has no player instance")
	}
	q := &FilmPlayback{Engine: c}
	v := NewReplayViewer(q)
	scene := p.Scene
	scene.Instances = append([]render3d.Instance(nil), p.instances...)
	scene.Camera = v.camera
	before := p.FilmCheckpoint()
	if int16(scene.Camera.Height) < 392 {
		t.Fatal("source carrier camera floor missing", scene.Camera)
	}
	corrected := p.Renderer.ObserverCamera(scene, focus)
	if corrected != scene.Camera {
		t.Fatal("legitimate carrier orbit altered", scene.Camera, corrected)
	}
	pixels := make([]byte, scene.TargetPixels())
	if _, err := p.Renderer.Draw(scene, &p.Visual, pixels); err != nil {
		t.Fatal(err)
	}
	without := scene
	without.Instances = nil
	for _, in := range scene.Instances {
		if in.ModelID != 64 {
			without.Instances = append(without.Instances, in)
		}
	}
	absent := make([]byte, len(pixels))
	if _, err := p.Renderer.Draw(without, &p.Visual, absent); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(pixels, absent) {
		t.Fatal("carrier geometry was not visibly exercised")
	}
	scenes := map[string]render3d.Scene{"legitimate-carrier": scene}
	close := scene
	close.Instances = []render3d.Instance{focus}
	close.Camera = render3d.Camera{Pose: render3d.Pose{H0: focus.H0, H1: focus.H1 - 512, Height: focus.Height + 80}, Angles: [3]uint16{0xfc18, 0, 0}}
	if p.Renderer.ObserverCamera(close, focus) != close.Camera {
		t.Fatal("close aircraft view moved")
	}
	scenes["legitimate-close-helicopter"] = close
	for name, s := range scenes {
		a, b := make([]byte, s.TargetPixels()), make([]byte, s.TargetPixels())
		visual, _ := p.presentationVisual(s, 0, false)
		if _, err := p.Renderer.Draw(s, &visual, a); err != nil {
			t.Fatal(err)
		}
		s.Camera = p.Renderer.ObserverCamera(s, focus)
		if _, err := p.Renderer.Draw(s, &visual, b); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatal("legitimate view pixels changed", name)
		}
		if out := os.Getenv("GS2000_R14_CAPTURE"); out != "" {
			if e := os.MkdirAll(out, 0700); e != nil {
				t.Fatal(e)
			}
			img, e := indexedFrameRGBA(a, p.Palette)
			if e != nil {
				t.Fatal(e)
			}
			fp, e := os.Create(filepath.Join(out, name+".png"))
			if e != nil {
				t.Fatal(e)
			}
			e = png.Encode(fp, img)
			fp.Close()
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
		t.Fatal("observer controls changed checkpoint")
	}
}
