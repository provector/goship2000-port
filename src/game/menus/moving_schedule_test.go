package menus

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func movingController(t *testing.T, scale, fps int, mode string) *FrontEnd {
	t.Helper()
	c := authoredUIController(t)
	c.Runtime = c.Runtime.Effective()
	c.Runtime.SimulationRenderScale, c.Runtime.PresentationFPS, c.Runtime.PresentationMode = scale, fps, mode
	doc := authorThroughUI(t, c)
	saved, rev, e := c.Files.Store.Load(doc.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.editMissionDocument(saved, rev); e != nil {
		t.Fatal(e)
	}
	if !c.Editor.Formation {
		editorClick(t, c, 145, 180)
	}
	editorClick(t, c, 34, 160)
	index := -1
	for i, o := range c.Editor.Options {
		if o.Formation == 47 {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("source EURO6 moving formation47 missing")
	}
	a := c.Editor.popupArea(index, len(c.Editor.Options))
	editorClick(t, c, a[0]+2, a[1]+2)
	editorClick(t, c, 45, 48)
	editorAction(t, c, Actions{Cancel: true})
	editorKey(t, c, "p")
	a = c.Editor.popupArea(2, 8)
	editorClick(t, c, a[0]+2, a[1]+2)
	editorClick(t, c, 45, 48)
	editorClick(t, c, 66, 48)
	if c.Editor.Held {
		t.Fatal("source route destination not accepted", c.Editor.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	editorKey(t, c, "b")
	for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
		if e = c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal("real authored BEGIN", c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	return c
}

func TestMovingActorsUseLogicalVisitsNotPresentation(t *testing.T) {
	for _, rate := range []int{1, 2} {
		var baseline []byte
		for _, q := range []struct {
			scale, fps int
			mode       string
		}{{1, 30, "stepped"}, {2, 60, "smooth"}} {
			t.Run(fmt.Sprintf("rate%d/scale%d/%s%d", rate, q.scale, q.mode, q.fps), func(t *testing.T) {
				c := movingController(t, q.scale, q.fps, q.mode)
				p := c.Rendering
				header := c.Mission.Data.Objectives["primary"].ObjectIndex
				if p.Initialized.Entities[header].Kind != 0x40 {
					t.Fatal("actual moving header absent")
				}
				first := p.Initialized.Entities[header+1].Pose
				if rate == 2 {
					editorAction(t, c, Actions{CompressionIncrease: true})
					editorAction(t, c, Actions{})
				}
				snapshot := func() []byte {
					return filmJSON(struct {
						Entities, Route any
						Random          uint16
					}{p.Initialized.Entities, p.Flight.Combat.FilmState().Route, p.Initialized.RandomState})
				}
				var previous time.Duration
				for frame := 1; frame <= q.fps*2; frame++ {
					now := time.Duration(frame) * time.Second / time.Duration(q.fps)
					if e := c.AdvanceRendering(float64(now-previous) / float64(time.Second)); e != nil {
						t.Fatal(e)
					}
					previous = now
					if c.Screen != ScreenScene || p.Flight.Failure != "" {
						t.Fatal("unplanned source stop", c.Screen, p.Flight.Failure)
					}
					before := snapshot()
					p.Dirty = true
					c.Frame(false)
					p.Dirty = true
					c.Frame(false)
					if !bytes.Equal(before, snapshot()) {
						t.Fatal("redraw repeated actor motion/RNG")
					}
				}
				if p.Flight.Steps != uint64(20*rate) || p.Initialized.Entities[header+1].Pose == first {
					t.Fatal("actual moving child/logical visit count", p.Flight.Steps, first, p.Initialized.Entities[header+1].Pose)
				}
				out := snapshot()
				if baseline == nil {
					baseline = out
				} else if !bytes.Equal(baseline, out) {
					t.Fatal("scale/FPS/mode changed source actor positions/dynamic/RNG")
				}
				before := snapshot()
				steps := p.Flight.Steps
				editorAction(t, c, Actions{GPSOpen: true})
				if p.GPS == nil {
					t.Fatal("GPS entry")
				}
				for n := 0; n < 12; n++ {
					if e := c.tickGPS(); e != nil {
						t.Fatal(e)
					}
					c.Frame(false)
				}
				if p.Flight.Steps != steps || !bytes.Equal(before, snapshot()) {
					t.Fatal("GPS/modal redraw advanced moving actors")
				}
				t.Logf("actual authored PATH%d header%d child%d;2host seconds rate%d -> %d logical visits; entities/route/RNG=%x", len(c.Mission.Data.Paths), header, header+1, rate, steps, sha256.Sum256(out))
				c.AbandonFilmRecording()
			})
		}
	}
}
