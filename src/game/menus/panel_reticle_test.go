package menus

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"testing"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/configuration"
)

type panelRun struct {
	player  *FilmPlayback
	bounds  cockpit.Rect
	targets [2]uint16
}

func startPanelRun(t *testing.T, res string, floating bool) panelRun {
	t.Helper()
	a, _ := benchmarkScenes(t)
	c := configuration.Defaults()
	c.SimulationResolution, c.FloatingGeometry = res, floating
	q, err := NewFilmPlayback(a, rendererFilm(t), c)
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
	cockpitTarget(p)
	family, err := cockpit.Family(p.Cockpit.AircraftID)
	if err != nil {
		t.Fatal(err)
	}
	d := a.Cockpit.Definitions[family].Parameters
	r := panelRun{player: q, bounds: cockpit.Rect{Left: int(d[0x54/2]), Top: int(d[0x56/2]), Right: int(d[0x58/2]), Bottom: int(d[0x5a/2])}}
	r.targets[0] = p.Flight.Avionics.Target
	for i, e := range p.Initialized.Entities {
		if e.ID == p.Initialized.PlayerID || uint16(i+1) == r.targets[0] {
			continue
		}
		for _, obj := range p.instances {
			if obj.ID == e.ID {
				r.targets[1] = uint16(i + 1)
				return r
			}
		}
	}
	t.Fatal("fixture needs two acquired target candidates")
	return r
}

const panelFrames = 9

func (r panelRun) move(j int) {
	p, b := r.player.Preview(), r.bounds
	x, y := (b.Left+b.Right)/2, b.Top
	path := [panelFrames]image.Point{{b.Left - 25, y - 15}, {x, y - 14}, {x, y - 5}, {b.Right - 5, y}, {b.Right, y + 3}, {b.Right + 7, y + 6}, {x, y + 13}, {x, y + 15}, {b.Right + 25, y + 8}}
	p.Flight.Avionics.Target = r.targets[j/5]
	p.Flight.Avionics.TargetPanelZoom = uint16(256 + j*8)
	p.Cockpit.TargetX = int16(path[j].X - 160)
	p.Cockpit.TargetY = int16(path[j].Y - (y - 35))
	p.Cockpit.TargetSymbol = 1 + j/5
	p.Cockpit.TargetAim = &cockpit.TargetAimInputs{TargetActive: true, CenterX: 160, CenterY: int16(y - 35), ClipBottom: 199, RadiusAdjust: 32}
	// Use the larger near-target circle at the upper/right edge as in the incident.
	if j >= 2 && j <= 5 {
		p.Cockpit.TargetAim.RadiusAdjust = 0
	}
	p.Cockpit.HUD = true
	p.Dirty = true
}

// Frozen V4 composition at independently calculated terrain-cleared camera
// poses. The original V2.1 sight and panel ownership are part of each digest.
// See Evidence/StepV42/harness/oracle.py.
func TestMovingReticleAcrossTargetPanel(t *testing.T) {
	for _, tc := range []struct {
		res      string
		floating bool
		want     string
	}{
		{"640x480", true, "bc15b13ebad0f5b62fb17f5c3a6329c5ac748a21ecb52a7e8af80e385370fdbe"}, {"1280x1024", true, "dd53e66d5b65a6b4fe1b72b5693f798e2863bb35d8bb0ef40f3eafbfd2f91773"}, {"640x480", false, "dc9d09ad2539489af040659f88fd9a24e73c631c38a17f362a8c8585121b423c"},
	} {
		r := startPanelRun(t, tc.res, tc.floating)
		// These frozen full-frame hashes include the historical foliage fill.
		// The normal display is checked against this frame's visible leaf writes below.
		r.player.Preview().Scene.SourceFillReference = true
		r.player.Preview().presentation.referenceDirty = true
		var rgba *image.RGBA
		h := sha256.New()
		for j := 0; j < panelFrames; j++ {
			r.move(j)
			p := r.player.Preview()
			before := p.FilmCheckpoint()
			var err error
			rgba, err = r.player.Engine.renderRGBA(false, rgba)
			if err != nil {
				t.Fatal(err)
			}
			if d := compareFilmCheckpoint(before, p.FilmCheckpoint()); d != "" {
				t.Fatal(d)
			}
			h.Write(p.Frame)
			h.Write(p.skyMask)
			h.Write(rgba.Pix)
		}
		if got := fmt.Sprintf("%x", h.Sum(nil)); got != tc.want {
			t.Errorf("%s float=%t motion/ownership hash %s; want %s", tc.res, tc.floating, got, tc.want)
		}
	}
}

// Two solid marker fills identify exactly which final cockpit pixels were
// written by scenery leaves even when the original palette index matched.
func TestMovingReticleWithDisplayFoliage(t *testing.T) {
	for _, mode := range []struct {
		res      string
		floating bool
	}{{"640x480", true}, {"1280x1024", true}, {"640x480", false}} {
		var runs [4]panelRun
		for i := range runs {
			runs[i] = startPanelRun(t, mode.res, mode.floating)
			p := runs[i].player.Preview()
			p.Scene.SourceFillReference = i == 0
			p.presentation.referenceDirty = true
			if i >= 2 {
				for _, g := range p.Renderer.Assets.Geometries {
					if g.CompactCallback != 1 && g.CompactCallback != 4 && g.CompactCallback != 5 {
						continue
					}
					for _, group := range g.DirectGroups {
						q := group.Primitive
						if q != nil && q.Colour>>8 == 0 && (q.Opcode == 3 || q.Opcode == 1) {
							q.Colour = uint16(0xe1+i-2) * 0x101
						}
					}
				}
			}
		}
		changed, panelChanged := 0, 0
		for j := 0; j < panelFrames; j++ {
			for _, run := range runs {
				run.move(j)
				if _, err := run.player.Engine.RenderRGBA(false); err != nil {
					t.Fatal(err)
				}
			}
			ref, display := runs[0].player.Preview(), runs[1].player.Preview()
			a, b := runs[2].player.Preview(), runs[3].player.Preview()
			if !bytes.Equal(ref.skyMask, display.skyMask) {
				t.Fatal("cockpit ownership differs", mode, j)
			}
			if d := compareFilmCheckpoint(ref.FilmCheckpoint(), display.FilmCheckpoint()); d != "" {
				t.Fatal(j, d)
			}
			bounds, l := runs[0].bounds, ref.Scene.Layout()
			for i, v := range display.Frame {
				if v == ref.Frame[i] {
					continue
				}
				if a.Frame[i] == b.Frame[i] || v>>2 != ref.Frame[i]>>2 {
					t.Fatal("changed a non-foliage cockpit pixel", mode, j, i)
				}
				changed++
				x, y := i%l.Width, i/l.Width
				if x >= l.X[bounds.Left] && x < l.X[bounds.Right+1] && y >= l.Y[bounds.Top] && y < l.Y[bounds.Bottom+1] {
					panelChanged++
				}
			}
		}
		if changed == 0 {
			t.Fatal("fixture no longer contains visible leaves", mode)
		}
		t.Logf("%s float=%t cockpit changed=%d including target panel=%d", mode.res, mode.floating, changed, panelChanged)
	}
}
