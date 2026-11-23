package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"testing"
)

func testStartupAssets() *assetdata.StartupAssets {
	palette := make([]byte, 768)
	palette[3] = 63
	pixels := make([]byte, assetdata.StartupPixels)
	pixels[0] = 1
	frame := assetdata.StartupFrame{Pixels: pixels, PaletteRGB6: palette, HoldSeconds: 10}
	return &assetdata.StartupAssets{Stages: []*assetdata.StartupAnimation{{Name: "logo", Frames: []assetdata.StartupFrame{frame}}, {Name: "intro and embedded credits", Frames: []assetdata.StartupFrame{frame}}}}
}

func TestPlaybackBeforeDelaysZeroRecordsAndBoundedCatchup(t *testing.T) {
	assets := testStartupAssets()
	a := assets.Stages[0].Frames[0]
	a.DelayBeforeSeconds = .1
	a.HoldSeconds = 0
	b := a
	b.DelayBeforeSeconds = 0
	d := a
	d.DelayBeforeSeconds = .2
	d.HoldSeconds = .1
	assets.Stages[0].Frames = []assetdata.StartupFrame{a, b, d}
	p, err := NewStartupPlayback(assets)
	if err != nil {
		t.Fatal(err)
	}
	p.Advance(.099)
	if p.Frame != -1 {
		t.Fatal("delay-before was applied after blit")
	}
	p.Advance(.001)
	if p.Frame != 1 {
		t.Fatal("zero-delay records lost order", p.Frame)
	}
	p.Advance(.2)
	if p.Frame != 2 {
		t.Fatal(p.Frame)
	}
	p.Advance(.1)
	if p.Stage != 1 || p.Frame != 0 {
		t.Fatal(p.Stage, p.Frame)
	}
	before := p.remaining
	p.Advance(1000)
	if p.remaining != before-.25 {
		t.Fatal("unbounded catchup")
	}
}

func TestStartupSkipEdgesMouseAndQuitOverlay(t *testing.T) {
	c := sourceFrontEnd(t, true)
	if err := c.BeginStartup(testStartupAssets()); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceStartup(0); err != nil {
		t.Fatal(err)
	}
	// A mouse Selector1 does not mean Space's stage-skip action.
	act(t, c, Actions{Selector1: true, X: 184, Y: 70, Inside: true})
	if c.Startup.Stage != 0 {
		t.Fatal("mouse skipped startup")
	}
	before := c.Frame(false)
	state := c.Store.State()
	act(t, c, Actions{Quit: true, Text: []rune{'q'}})
	if err := c.AdvanceStartup(1); err != nil {
		t.Fatal(err)
	}
	if c.Startup.Frame != 0 {
		t.Fatal("quit advanced playback")
	}
	rgba, err := c.RenderRGBA(false)
	if err != nil {
		t.Fatal(err)
	}
	if rgba.RGBAAt(0, 0).R != 255 || rgba.RGBAAt(0, 0).G != 0 {
		t.Fatal("quit recoloured the background")
	}
	indexed := c.Frame(false)
	greens := 0
	for y := 92; y < 100; y++ {
		for x := 90; x < 230; x++ {
			if indexed[y*320+x] == 10 {
				greens++
				if rgba.At(x, y) != c.Office.Assets.Palette[10] {
					ar, ag, ab, _ := rgba.At(x, y).RGBA()
					br, bg, bb, _ := c.Office.Assets.Palette[10].RGBA()
					if ar != br || ag != bg || ab != bb {
						t.Fatal("quit green used startup palette")
					}
				}
			}
		}
	}
	if greens == 0 {
		t.Fatal("no quit green text")
	}
	act(t, c, Actions{AnyKey: true})
	if c.Screen != ScreenStartup || !bytes.Equal(before, c.Frame(false)) || c.Store.State().Revision != state.Revision {
		t.Fatal("quit dismissal changed startup/save")
	}
	latch := inputLatch{}
	a := latch.Read(Buttons{Space: true, AnyKeyDown: true})
	act(t, c, a)
	if c.Startup.Stage != 1 {
		t.Fatal("Space did not skip logo")
	}
	act(t, c, latch.Read(Buttons{Space: true, AnyKeyDown: true}))
	if c.Screen != ScreenStartup {
		t.Fatal("held Space skipped two stages")
	}
	act(t, c, latch.Read(Buttons{}))
	act(t, c, latch.Read(Buttons{Space: true, AnyKeyDown: true}))
	if c.Screen != ScreenOffice {
		t.Fatal("second fresh Space did not reach office")
	}
	act(t, c, latch.Read(Buttons{Space: true, AnyKeyDown: true, X: 184, Y: 70, Inside: true}))
	if c.Screen != ScreenOffice {
		t.Fatal("skip press activated office hotspot")
	}
}

func TestQuitDeclineTextIsSuppressedUntilRelease(t *testing.T) {
	latch := inputLatch{}
	latch.Read(Buttons{Text: []rune{'n'}, AnyKeyPressed: true, AnyKeyDown: true})
	latch.SuppressTextUntilRelease()
	if a := latch.Read(Buttons{Text: []rune{'n'}, AnyKeyDown: true}); len(a.Text) != 0 {
		t.Fatal("held response leaked to editor")
	}
	latch.Read(Buttons{})
	if a := latch.Read(Buttons{Text: []rune{'X'}, AnyKeyPressed: true, AnyKeyDown: true}); string(a.Text) != "X" {
		t.Fatal("fresh text remained suppressed")
	}
}

func TestPreparedStartupWhenRequested(t *testing.T) {
	root := os.Getenv("GS2000_TEST_STARTUP")
	if root == "" {
		t.Skip("set GS2000_TEST_STARTUP for prepared original-source animation checks")
	}
	a, err := assetdata.LoadStartupAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Stages) != 2 || len(a.Stages[0].Frames) != 82 || len(a.Stages[1].Frames) != 1114 {
		t.Fatal("source/presentation frame counts")
	}
	p, err := NewStartupPlayback(a)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 6000 && !p.Done; n++ {
		p.Advance(1.0 / 70)
	}
	if !p.Done {
		t.Fatal("natural playback did not terminate")
	}
}

func TestStartupCaptureOwnership(t *testing.T) {
	c := sourceFrontEnd(t, true)
	animation := testStartupAssets()
	if err := c.BeginStartup(animation); err != nil {
		t.Fatal(err)
	}
	c.Startup.Advance(0)
	public := c.Frame(false)
	capture, err := c.RenderRGBA(false)
	if err != nil {
		t.Fatal(err)
	}
	public[0] = 0
	if animation.Stages[0].Frames[0].Pixels[0] != 1 {
		t.Fatal("indexed capture changed intro data")
	}
	animation.Stages[0].Frames[0].Pixels[0] = 0
	next, err := c.RenderRGBA(false)
	if err != nil || capture.RGBAAt(0, 0).R != 255 || next.RGBAAt(0, 0).R != 0 {
		t.Fatal("RGBA capture did not retain its original colour", err)
	}
}
