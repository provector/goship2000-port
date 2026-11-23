package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
	"time"
)

func resolutionCapture(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	out := os.Getenv("GS2000_U1_OUTPUT")
	if out == "" {
		return
	}
	f, e := os.Create(filepath.Join(out, name+".png"))
	if e != nil {
		t.Fatal(e)
	}
	e = png.Encode(f, img)
	ce := f.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
}

func TestSceneMotion(t *testing.T) {
	assets, scenes := benchmarkScenes(t)
	r, e := render3d.New(assets.World)
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"320x200", "640x400", "640x480", "800x600", "1024x768", "1280x1024"} {
		for _, name := range []string{"euro6-pad", "euro8-field", "euro-trees", "jungle", "gulf", "antarctic"} {
			s := scenes[name]
			s.Resolution = mode
			w, h := s.Dimensions()
			dst := make([]byte, w*h)
			s.SkyMask = make([]byte, w*h)
			var prev [32]byte
			for i := 0; i < 4; i++ {
				s.Camera.H0 += uint32(i * 16)
				s.Camera.Angles[1] += uint16(48 * i)
				s.Camera.Angles[2] += uint16(64 * i)
				before, _ := json.Marshal(s)
				pal, err := r.Palette(s, nil)
				if err != nil {
					t.Fatal(err)
				}
				p := &RenderingPreview{Scene: s, Frame: dst, Renderer: r, Palette: pal, skyMask: s.SkyMask}
				c := FrontEnd{Office: &Office{Assets: assets}, Screen: ScreenScene, Rendering: p}
				var first [32]byte
				for n := 0; n < 2; n++ {
					clear(dst)
					if _, err = r.Draw(s, nil, dst); err != nil {
						t.Fatal(err)
					}
					im, err := c.RenderRGBA(false)
					if err != nil {
						t.Fatal(err)
					}
					if im.Rect.Dx() != w || im.Rect.Dy() != h {
						t.Fatal("frame dimensions", mode, im.Rect)
					}
					digest := sha256.Sum256(im.Pix)
					if n == 0 {
						first = digest
						if i == 0 || name == "euro6-pad" || name == "jungle" {
							resolutionCapture(t, fmt.Sprintf("%s-%s-%d", name, mode, i), im)
						}
					} else if first != digest {
						t.Fatal("unstable", name, mode, i)
					}
				}
				if i == 3 && first == prev && name == "euro6-pad" {
					t.Fatal("fixture motion had no effect")
				}
				prev = first
				after, _ := json.Marshal(s)
				if !bytes.Equal(before, after) {
					t.Fatal("scene mutation")
				}
			}
		}
	}
}

func cockpitTarget(p *RenderingPreview) {
	// Same acquired aircraft, zoom, view and camera for every timing sample.
	for i, entity := range p.Initialized.Entities {
		if entity.ID != p.Initialized.PlayerID {
			for _, instance := range p.instances {
				if instance.ID == entity.ID {
					p.Flight.Avionics.Target = uint16(i + 1)
					break
				}
			}
			if p.Flight.Avionics.Target != 0 {
				break
			}
		}
	}
	p.Cockpit.PrimaryMFD = 5
	p.Cockpit.CameraMode = 0
	p.Cockpit.Damage[0] &^= 0x20
	p.Cockpit.Arrangement = 0
	p.Flight.Avionics.TargetPanelZoom = 256
	p.Scene.GraphicsOptions |= 8
	p.presentation.referenceDirty = true
}

func TestPlaybackAndCockpit(t *testing.T) {
	path := os.Getenv("GS2000_R7_FILM")
	if path == "" {
		t.Skip("requires supported R7 cockpit film")
	}
	assets, _ := benchmarkScenes(t)
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	film, e := replay.Unmarshal(raw)
	if e != nil {
		t.Fatal(e)
	}
	var reference map[string]string
	for _, mode := range []string{"320x200", "640x400", "640x480", "800x600", "1024x768", "1280x1024"} {
		cfg := configuration.Defaults()
		cfg.SimulationResolution = mode
		q, e := NewFilmPlayback(assets, film, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for q.reachedSteps < 16 && !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
		p := q.Preview()
		if reference == nil {
			reference = p.FilmCheckpoint()
		} else if diff := compareFilmCheckpoint(reference, p.FilmCheckpoint()); diff != "" {
			t.Fatal(mode, diff)
		}
		viewer := &FrontEnd{Office: q.Engine.Office, Viewer: &ReplayViewer{Playback: q}, Screen: ScreenReplay}
		rendered := viewer.Frame(false)
		if len(rendered) != p.Scene.TargetPixels() {
			t.Fatal("viewer dimensions")
		}
		if e = q.Resume(); e != nil {
			t.Fatal(mode, e)
		}
		c := q.Engine
		p = q.Preview()
		w, h := p.Scene.Dimensions()
		var before map[string]string
		cockpitTarget(p)
		before = p.FilmCheckpoint()
		var img *image.RGBA
		draw := func() {
			p.Dirty = true
			img, e = c.renderRGBA(false, img)
			if e != nil {
				t.Fatal(e)
			}
		}
		draw()
		if img.Rect.Dx() != w || img.Rect.Dy() != h || len(p.targetFrame) != w*h {
			t.Fatal("target MFD did not render", mode, img.Rect, len(p.targetFrame))
		}
		resolutionCapture(t, "cockpit-"+mode, img)
		layout := p.Scene.Layout()
		family, _ := cockpit.Family(p.Cockpit.AircraftID)
		params := assets.Cockpit.Definitions[family].Parameters
		t.Logf("view %s world=%s instances=%d main=%dx%d mfd=%dx%d retained_indexed_bytes=%d rgba_bytes=%d", mode, p.Scene.World, len(p.Scene.Instances), w, layout.Y[p.Scene.View.Bounds.Bottom+1]-layout.Top, layout.X[params[0x58/2]+1]-layout.X[params[0x54/2]], layout.Y[params[0x5a/2]+1]-layout.Y[params[0x56/2]], cap(p.Frame)+cap(p.worldFrame)+cap(p.worldSkyMask)+cap(p.skyMask)+cap(p.presentation.referenceBase)+cap(p.targetBrownMask)+cap(p.targetFrame), len(img.Pix))
		for i := 0; i < 3; i++ {
			draw()
		}
		runtime.GC()
		var start, end runtime.MemStats
		runtime.ReadMemStats(&start)
		var ms [25]float64
		for i := range ms {
			now := time.Now()
			draw()
			ms[i] = float64(time.Since(now)) / 1e6
		}
		runtime.ReadMemStats(&end)
		slices.Sort(ms[:])
		t.Logf("timing %s n=25 median_ms=%.2f p95_ms=%.2f max_ms=%.2f allocations/frame=%d bytes/frame=%d target_pixels=%d", mode, ms[12], ms[23], ms[24], (end.Mallocs-start.Mallocs)/25, (end.TotalAlloc-start.TotalAlloc)/25, w*h)
		if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
			t.Fatal(mode, "redraw changed authoritative state")
		}
		for _, camera := range []int{7, 3, 4, 5, 6} {
			p.Cockpit.CameraMode = camera
			p.presentation.referenceDirty = true
			draw()
			if img.Rect.Dx() != w {
				t.Fatal("camera size")
			}
		}
		// Strict replay consumes each operation and validates the recorded ending.
		strict, e := NewFilmPlayback(assets, film, cfg)
		if e != nil {
			t.Fatal(e)
		}
		for !strict.Done {
			if e = strict.Step(); e != nil {
				t.Fatal(mode, strict.Cursor, e)
			}
		}
		t.Logf("strict %s checked %d operations", mode, strict.Cursor)

	}
}

func TestMousePictureMapping(t *testing.T) {
	for _, mode := range []string{"320x200", "640x400", "640x480", "800x600", "1024x768", "1280x1024"} {
		s := render3d.Scene{Resolution: mode}
		l := s.Layout()
		for _, win := range [][2]int{{1280, 1024}, {1900, 900}, {800, 600}} {
			v := officeViewport(win[0], win[1], false)
			for _, p := range [][2]int{{0, 0}, {319, 199}, {113, 82}, {250, 196}, {2, 3}} {
				x := float64(l.X[p[0]]) + 0.5
				y := float64(l.Y[p[1]]-l.Top) + 0.5
				gx, gy, ok := v.LogicalPoint(v.X+x*v.Width/float64(l.Width), v.Y+y*v.Height/float64(l.PictureHeight))
				if !ok || gx != p[0] || gy != p[1] {
					t.Fatal(mode, win, p, gx, gy, ok)
				}
			}
		}
	}
}

func TestCarrierAndRepeatedSizeChanges(t *testing.T) {
	c := mastFixture(t, 6, "carrier")
	jFinishFade(t, c)
	p := c.Rendering
	v := NewReplayViewer(&FilmPlayback{Engine: c})
	before := p.FilmCheckpoint()
	for i, mode := range []string{"800x600", "1280x1024", "1024x768", "320x200", "1280x1024", "800x600"} {
		opts := configuration.Defaults()
		opts.SimulationResolution = mode
		opts.FloatingGeometry = i%2 == 1
		applyReplayPresentationOptions(p, c, opts)
		if err := p.draw(c.Office.Assets.Cockpit); err != nil {
			t.Fatal(err)
		}
		if len(p.Frame) != p.Scene.TargetPixels() || len(p.presentation.referenceBase) != len(p.Frame) {
			t.Fatal("stale frame")
		}
		old := &p.Frame[0]
		applyReplayPresentationOptions(p, c, opts)
		if old != &p.Frame[0] {
			t.Fatal("unnecessary allocation")
		}
		s := p.Scene
		s.Camera = v.camera
		s.Instances = slices.Clone(p.instances)
		s.GroundOwners = nil
		s.SkyMask = nil
		dst := make([]byte, s.TargetPixels())
		if _, err := p.Renderer.Draw(s, &p.Visual, dst); err != nil {
			t.Fatal(err)
		}
		s.Instances = slices.DeleteFunc(s.Instances, func(i render3d.Instance) bool { return i.ModelID == 64 })
		other := make([]byte, len(dst))
		if _, err := p.Renderer.Draw(s, &p.Visual, other); err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(dst, other) {
			t.Fatal(mode, "carrier was not visible")
		}
		if diff := compareFilmCheckpoint(before, p.FilmCheckpoint()); diff != "" {
			t.Fatal("resize changed", diff)
		}
	}
}
