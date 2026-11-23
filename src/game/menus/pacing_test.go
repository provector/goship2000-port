package menus

import (
	"testing"

	"gunship2000-assets/game/configuration"
)

func TestFramerateChoicesKeepFlightClock(t *testing.T) {
	assets, _ := benchmarkScenes(t)
	film := rendererFilm(t)
	var baseline map[string]string
	for _, modern := range []bool{false, true} {
		opts := configuration.DefaultConfig().Runtime
		if !modern {
			opts.SimulationResolution, opts.SimulationRenderScale = "320x200", 1
			opts.PresentationMode, opts.FloatingGeometry = "stepped", false
		}
		p, err := NewFilmPlayback(assets, film, opts)
		if err != nil {
			t.Fatal(err)
		}
		for p.reachedSteps < 16 {
			if err = p.Step(); err != nil {
				t.Fatal(err)
			}
		}
		if err = p.Resume(); err != nil {
			t.Fatal(err)
		}
		engine := p.Engine
		start := p.Preview().Flight.Steps
		for tick := 0; tick < 60; tick++ {
			if tick == 12 {
				act(t, engine, Actions{CompressionIncrease: true})
			}
			if err = engine.AdvanceRendering(1.0 / 60); err != nil {
				t.Fatal(err)
			}
			if _, err = engine.RenderRGBA(false); err != nil {
				t.Fatal(err)
			}
		}
		if p.Preview().Flight.Steps <= start {
			t.Fatal("flight did not advance")
		}
		h := p.Preview().FilmCheckpoint()
		if baseline == nil {
			baseline = h
		} else if d := compareFilmCheckpoint(baseline, h); d != "" {
			t.Fatal(d)
		}
		t.Logf("%s: %d logical flight steps", opts.PresentationMode, p.Preview().Flight.Steps-start)
	}
}
