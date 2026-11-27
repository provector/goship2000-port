package menus

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"testing"
)

func TestPausedGraphics(t *testing.T) {
	run := startPanelRun(t, "640x480", true)
	c := run.player.Engine
	p := run.player.Preview()
	before := p.FilmCheckpoint()
	ref := bytes.Clone(p.avionicsReference)
	g := graphicsOptions{draft: displayPreferences{runtime: c.Runtime, window: displayWindow{width: 811, height: 599}}, dirty: true}
	for _, screen := range []Screen{ScreenOffice, ScreenStartup, ScreenMissionEditor, ScreenQuit} {
		mock := *c
		mock.Screen = screen
		if _, p := graphicsScene(&mock); p != nil {
			t.Fatal(screen)
		}
	}
	for _, r := range configuration.DisplayResolutions() {
		g.draft.runtime.SetResolution(r)
		applyDisplay(c, g.draft.runtime)
		g.dirty = true
		im, e := g.compose(c)
		if e != nil {
			t.Fatal(e)
		}
		if im.Rect.Dx() != p.Scene.Layout().Width {
			t.Fatal(r)
		}
		again, _ := g.compose(c)
		if again != im {
			t.Fatal("idle rebuild")
		}
		if d := compareFilmCheckpoint(before, p.FilmCheckpoint()); d != "" || !bytes.Equal(ref, p.avionicsReference) {
			t.Fatal(r, d)
		}
	}
	for _, row := range []int{1, 2, 3, 4} {
		g.row = row
		g.cycle(1)
	}
	if g.draft.runtime.FloatingGeometry || g.draft.runtime.PresentationMode != "stepped" || !g.draft.meter || g.draft.window.String() != "640x480" {
		t.Fatal(g.values())
	}
}

func TestReplayGraphicsReadOnlyUntilResume(t *testing.T) {
	a, _ := benchmarkScenes(t)
	opts := configuration.Defaults()
	opts.SetResolution("1024x768")
	opts.FloatingGeometry = false
	opts.SetFramerate("stepped")
	q, err := NewFilmPlayback(a, rendererFilm(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	outer := &FrontEnd{Office: q.Engine.Office, Screen: ScreenReplay, Runtime: opts, Viewer: NewReplayViewer(q)}
	check := func(practice bool) {
		t.Helper()
		p := outer.Viewer.Playback.Preview()
		if p.Scene.Resolution != "1024x768" || p.Scene.FloatingGeometry || p.presentation.mode != "stepped" {
			t.Fatal("viewer replaced accepted graphics")
		}
		if _, modal := graphicsScene(outer); (modal != nil) != practice {
			t.Fatal("graphics menu admission", practice)
		}
	}
	for restart := 0; restart < 2; restart++ {
		check(false)
		if err = outer.activateReplayControl(4); err != nil {
			t.Fatal(err)
		}
		for outer.Viewer.Playback.reachedSteps < 16 {
			if err = outer.Viewer.Playback.Step(); err != nil {
				t.Fatal(err)
			}
		}
		check(false)
		if restart == 0 {
			if err = outer.activateReplayControl(0); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The viewer map is a playback overlay. Resume opens the child cockpit.
	if err = outer.activateReplayControl(15); err != nil {
		t.Fatal(err)
	}
	if err = outer.activateReplayControl(16); err != nil {
		t.Fatal(err)
	}
	check(true)
}

func TestWindowTraversalAfterConstrainedPreview(t *testing.T) {
	g := graphicsOptions{row: 3, draft: displayPreferences{window: displayWindow{width: 1024, height: 768}}}
	g.cycle(1)
	g.draft.window.height = 831 // compositor constrained the requested 1280x1024.
	g.cycle(1)
	if g.draft.window.String() != configuration.BorderlessLabel {
		t.Fatal(g.values())
	}
	for _, delta := range []int{-1, 1} {
		for range 12 {
			g.cycle(delta)
		}
		if g.draft.window.String() != configuration.BorderlessLabel {
			t.Fatal(delta, g.values())
		}
	}
}
