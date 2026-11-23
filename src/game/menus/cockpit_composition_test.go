package menus

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
)

func rendererFilm(t *testing.T) *replay.Film {
	t.Helper()
	path := os.Getenv("GS2000_R7_FILM")
	if path == "" {
		t.Skip("requires the R7 EURO6 film fixture in supported storage")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := replay.Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCockpitComposition(t *testing.T) {
	assets, _ := benchmarkScenes(t)
	film := rendererFilm(t)
	measure := os.Getenv("GS2000_U2_TIMING") != ""
	for i, res := range []string{"320x200", "640x480", "800x600", "1024x768", "1280x1024"} {
		var checkpoint map[string]string
		var panels [2][32]byte
		for k := 0; k < 2; k++ {
			fp := (i+k)%2 == 1
			cfg := configuration.Defaults()
			cfg.SimulationResolution = res
			cfg.FloatingGeometry = fp
			q, err := NewFilmPlayback(assets, film, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for q.reachedSteps < 16 && !q.Done {
				if err = q.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if err = q.Resume(); err != nil {
				t.Fatal(err)
			}
			p := q.Preview()
			if p.Scene.FloatingGeometry != fp || q.Engine.filmIsolation.Mode != "practice" {
				t.Fatal("wrong local renderer/practice")
			}
			cockpitTarget(p)
			if p.Scene.World != "EURO6" || len(p.Scene.Instances) != 75 {
				t.Fatal("unmatched workload", p.Scene.World, len(p.Scene.Instances))
			}
			before := p.FilmCheckpoint()
			if checkpoint == nil {
				checkpoint = before
			} else if diff := compareFilmCheckpoint(before, checkpoint); diff != "" {
				t.Fatal(diff)
			}
			var img *image.RGBA
			draw := func() {
				p.Dirty = true
				img, err = q.Engine.renderRGBA(false, img)
				if err != nil {
					t.Fatal(err)
				}
			}
			draw()
			name := "integer"
			if fp {
				name = "float"
			}
			resolutionCapture(t, "cockpit-"+res+"-"+name, img)
			layout := p.Scene.Layout()
			fam, _ := cockpit.Family(p.Cockpit.AircraftID)
			params := assets.Cockpit.Definitions[fam].Parameters
			x0, x1 := layout.X[params[0x54/2]], layout.X[params[0x58/2]+1]
			y0, y1 := layout.Y[params[0x56/2]], layout.Y[params[0x5a/2]+1]
			if len(p.targetFrame) != layout.Width*layout.Height {
				t.Fatal("target view missing")
			}
			d := sha256.New()
			for y := y0; y < y1; y++ {
				d.Write(p.targetFrame[y*layout.Width+x0 : y*layout.Width+x1])
			}
			copy(panels[k][:], d.Sum(nil))
			imageHash := sha256.Sum256(img.Pix)
			for n := 0; n < 6; n++ {
				draw()
			}
			if imageHash != sha256.Sum256(img.Pix) {
				t.Fatal("stationary redraw changed", res, name)
			}
			if measure {
				var ms [100]float64
				runtime.GC()
				var start, end runtime.MemStats
				runtime.ReadMemStats(&start)
				for n := range ms {
					now := time.Now()
					draw()
					ms[n] = float64(time.Since(now)) / 1e6
				}
				runtime.ReadMemStats(&end)
				slices.Sort(ms[:])
				t.Logf("timing %s %s %.4f %.4f %.2f %.0f", res, name, (ms[49]+ms[50])/2, ms[94], float64(end.Mallocs-start.Mallocs)/100, float64(end.TotalAlloc-start.TotalAlloc)/100)
			}
			if diff := compareFilmCheckpoint(before, p.FilmCheckpoint()); diff != "" {
				t.Fatal("redraw changed", diff)
			}
			t.Logf("pixels %s %s %x panel=%dx%d %x", res, name, imageHash, x1-x0, y1-y0, panels[k])
			if res == "800x600" {
				for _, cam := range []int{3, 4, 5, 6, 7} {
					p.Cockpit.CameraMode = cam
					p.presentation.referenceDirty = true
					draw()
					resolutionCapture(t, fmt.Sprintf("view-%d-%s", cam, name), img)
				}
			}
		}
		if panels[0] == panels[1] {
			t.Fatal("active MFD did not distinguish backends", res)
		}
	}
}

func TestFilmAndPracticeEquality(t *testing.T) {
	a, _ := benchmarkScenes(t)
	film := rendererFilm(t)
	for _, practice := range []bool{false, true} {
		var runs [2]*FilmPlayback
		for i := range runs {
			opts := configuration.Defaults()
			opts.SimulationResolution = "800x600"
			opts.FloatingGeometry = i == 1
			p, err := NewFilmPlayback(a, film, opts)
			if err != nil {
				t.Fatal(err)
			}
			runs[i] = p
		}
		n := 0
		for !runs[0].Done {
			for _, q := range runs {
				if err := q.Step(); err != nil {
					t.Fatal(err)
				}
			}
			n++
			left, right := runs[0].Preview(), runs[1].Preview()
			if diff := compareFilmCheckpoint(left.FilmCheckpoint(), right.FilmCheckpoint()); diff != "" {
				t.Fatal(n, diff)
			}
			if !bytes.Equal(left.avionicsReference, right.avionicsReference) {
				t.Fatal(n, "avionics raster differs")
			}
			if runs[0].reachedSteps == 12 && !practice {
				var hashes [2][32]byte
				for i, q := range runs {
					v := NewReplayViewer(q)
					c := FrontEnd{Office: q.Engine.Office, Viewer: v}
					pixels := c.replayFrame(false)
					hashes[i] = sha256.Sum256(pixels)
					if len(pixels) != 800*600 || hashes[i] != sha256.Sum256(c.replayFrame(false)) {
						t.Fatal("stationary VCR view", i)
					}
					v.Demo = true
					if len(c.replayFrame(false)) != 800*600 {
						t.Fatal("DEMO composition", i)
					}
				}
				if hashes[0] == hashes[1] {
					t.Fatal("external VCR view ignored geometry setting")
				}
			}
			if practice && runs[0].reachedSteps >= 16 {
				for _, q := range runs {
					if err := q.Resume(); err != nil {
						t.Fatal(err)
					}
					if q.Engine.Store != nil || q.Engine.filmIsolation.Mode != "practice" {
						t.Fatal("career isolation")
					}
					if err := q.Engine.advanceFlight(.2); err != nil {
						t.Fatal(err)
					}
				}
				if diff := compareFilmCheckpoint(left.FilmCheckpoint(), right.FilmCheckpoint()); diff != "" {
					t.Fatal("practice", diff)
				}
				break
			}
		}
		if !practice && (!runs[1].Done || runs[1].Divergence != "") {
			t.Fatal("strict ending")
		}
		t.Logf("practice=%t operations=%d", practice, n)
	}
}

func TestFieldAndJungleMotion(t *testing.T) {
	a, scenes := benchmarkScenes(t)
	r, err := render3d.New(a.World)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"euro6-pad", "jungle", "euro-trees"} {
		for _, fp := range []bool{false, true} {
			scene := scenes[name]
			scene.Resolution = "800x600"
			scene.FloatingGeometry = fp
			dst := make([]byte, scene.TargetPixels())
			scene.SkyMask = make([]byte, len(dst))
			pal, err := r.Palette(scene, nil)
			if err != nil {
				t.Fatal(err)
			}
			p := &RenderingPreview{Scene: scene, Frame: dst, Renderer: r, Palette: pal, skyMask: scene.SkyMask}
			c := FrontEnd{Office: &Office{Assets: a}, Screen: ScreenScene, Rendering: p}
			label := "integer"
			if fp {
				label = "float"
			}
			for n := 0; n < 6; n++ {
				scene.Camera.H1 += 2
				scene.Camera.Angles[1] += 12
				scene.Camera.Angles[2] += 9
				var digest [32]byte
				for redraw := 0; redraw < 2; redraw++ {
					clear(dst)
					_, err := r.Draw(scene, nil, dst)
					if err != nil {
						t.Fatal(err)
					}
					img, err := c.RenderRGBA(false)
					if err != nil {
						t.Fatal(err)
					}
					if redraw == 0 {
						digest = sha256.Sum256(img.Pix)
						resolutionCapture(t, fmt.Sprintf("%s-%s-%02d", name, label, n), img)
					} else if digest != sha256.Sum256(img.Pix) {
						t.Fatal("unstable", name, label, n)
					}
				}
			}
		}
	}
}

func TestMastCameraAndFractionalSnapshots(t *testing.T) {
	c := mastFixture(t, 1, "land")
	jFinishFade(t, c)
	p := c.Rendering
	opts := configuration.Defaults()
	opts.SimulationResolution, opts.FloatingGeometry = "800x600", true
	applyReplayPresentationOptions(p, c, opts)
	act(t, c, Actions{ViewKey: 2})
	act(t, c, Actions{})
	if p.ViewControls.Mode != 7 {
		t.Fatal("mast not selected")
	}
	for i := 1; i <= 18; i++ {
		act(t, c, Actions{InputTime: time.Duration(i) * time.Second / 60, FlightHeldValid: true, SceneDirections: [4]bool{true}})
		if err := c.AdvanceRendering(1.0 / 60); err != nil {
			t.Fatal(err)
		}
	}
	checkpoint := p.FilmCheckpoint()
	p.Dirty = true
	img, err := c.RenderRGBA(false)
	if err != nil || p.Error != nil {
		t.Fatal(err, p.Error)
	}
	resolutionCapture(t, "mast-pan-float", img)
	if diff := compareFilmCheckpoint(checkpoint, p.FilmCheckpoint()); diff != "" {
		t.Fatal(diff)
	}
	// Supply adjacent reached samples. Both alphas select the same integer
	// position and angle; only the floating submission can distinguish them.
	p.resetPresentation()
	p.presentation.current.Camera.H0 = p.presentation.previous.Camera.H0 + 1
	p.presentation.current.Camera.Angles[1] = p.presentation.previous.Camera.Angles[1] + 1
	for _, fp := range []bool{false, true} {
		p.Scene.FloatingGeometry = fp
		for _, alpha := range []float64{.125, .375} {
			p.presentation.alpha = alpha
			scene, _ := p.presentationInputs()
			if scene.Camera != p.presentation.previous.Camera {
				t.Fatal("rounded endpoint changed")
			}
			want := 0.0
			if fp {
				want = alpha
			}
			if scene.CameraFraction.Position[0] != want || scene.CameraFraction.Angles[1] != want {
				t.Fatal("fraction lost before transform", alpha, scene.CameraFraction)
			}
		}
	}
}
