package menus

import (
	"bytes"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestTerminalFinalJumpSkipsLateFadeButKeepsNoiseAudio(t *testing.T) {
	for _, count := range []int16{2, 1} {
		s := &flight.State{Terminal: &flight.Terminal{Status: 0x10, Countdown: count}}
		var p terminalPresentation
		draws := 0
		audio, err := p.advance(s, terminalPresentationInputs{GraphicsOptions: 0x10, AudioEnabled: true, Random: func() uint16 { draws++; return 327 }})
		if err != nil || len(audio) != 2 || audio[0].event != 0xb {
			t.Fatalf("source pre-jump audio %v %v", audio, err)
		}
		if draws != 32767/int(count) || p.Fade != (count != 1) || p.Completed != (count == 1) {
			t.Fatalf("final longjmp order: %+v", p)
		}
	}
}

func TestTerminalDescriptorAndLongjmpOriginalBytes(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("original operand check")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Static file coordinates, not loaded CS. Source23b0/countdown DEC's
	// zero arm calls the CRT saved-context transfer before late2547 palette work.
	at := 0xd8b0 + 0x2530
	if !bytes.Equal(raw[at:at+5], []byte{0x26, 0xff, 0x0e, 0x0c, 0x2d}) {
		t.Fatal("DEC source changed")
	}
	// Placement header owns callback30e2; no model96 lookup is performed.
	ds := 0x61bc0
	if !bytes.Equal(raw[ds+0x5c:ds+0x5e], []byte{0xe2, 0x30}) || !bytes.Equal(raw[ds+0x68:ds+0x6a], []byte{0x48, 0}) {
		t.Fatal("header/descriptor mapping changed")
	}
}

func TestExteriorTerminalPublishesOrderedCallbackAtBothScales(t *testing.T) {
	for _, scale := range []int{1, 2} {
		c := developmentScenarioControllerProfile(t, "crash", replay.V45Compatibility)
		p := c.Rendering
		p.Scene.RenderScale = scale
		p.resetPresentation()
		if err := c.Handle(Actions{Cancel: true}); err != nil {
			t.Fatal(err)
		}
		if err := c.Handle(Actions{}); err != nil {
			t.Fatal(err)
		}
		if err := c.Handle(Actions{ViewKey: 5}); err != nil {
			t.Fatal(err)
		}
		if err := c.AdvanceRendering(.12); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, in := range p.Scene.Instances {
			if in.ID == "terminal-header0048" {
				found = true
				if !in.Impact || in.GeometryID != "" {
					t.Fatal("invented terminal mesh")
				}
			}
		}
		published := false
		for _, in := range p.presentation.current.Instances {
			if in.ID == "terminal-header0048" {
				published = true
			}
		}
		if !found || !published || !p.Flight.TerminalPresentation.Exterior {
			t.Fatal("nonfinal callback failed to publish", scale, found, published, p.ViewControls.Mode)
		}
		a := c.Frame(false)
		b := c.Frame(false)
		if p.Error != nil || !bytes.Equal(a, b) {
			t.Fatal("terminal redraw ownership", p.Error)
		}
	}
}
