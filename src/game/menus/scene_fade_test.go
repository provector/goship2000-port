package menus

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestSceneFadeLegsPreserveSource(t *testing.T) {
	c := &FrontEnd{FadeMilliseconds: 150}
	from, to := image.NewRGBA(image.Rect(0, 0, 1, 1)), image.NewRGBA(image.Rect(0, 0, 1, 1))
	from.SetRGBA(0, 0, color.RGBA{200, 100, 50, 255})
	to.SetRGBA(0, 0, color.RGBA{80, 160, 240, 255})
	c.BeginSceneFade(from)
	f := c.fade
	if f.LegTicks != 11 {
		t.Fatal(f.LegTicks)
	}
	if got := c.applySceneFade(to); !bytes.Equal(got.Pix, from.Pix) {
		t.Fatal("outgoing source")
	}
	f.Tick = f.LegTicks
	if got := c.applySceneFade(to).RGBAAt(0, 0); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatal(got)
	}
	f.Tick = f.duration()
	if got := c.applySceneFade(to); !bytes.Equal(got.Pix, to.Pix) {
		t.Fatal("destination")
	}
	if from.RGBAAt(0, 0).R != 200 || to.RGBAAt(0, 0).R != 80 {
		t.Fatal("source mutation")
	}
}

func TestMissionFadesFreezeInputSessionAndQuit(t *testing.T) {
	c := missionController(t)
	c.FadeMilliseconds = 150
	if err := c.enterMission(); err != nil {
		t.Fatal(err)
	}
	s := c.Mission
	hash := s.Data.ObjectHash
	draws := len(s.Data.Draws)
	if c.fade == nil || c.fade.From == nil {
		t.Fatal("missing departure fade")
	}
	for i := 0; i < 4; i++ {
		_ = c.Tick()
	}
	tick := c.fade.Tick
	if err := c.Handle(Actions{Quit: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		_ = c.Tick()
	}
	if c.fade.Tick != tick {
		t.Fatal("quit advanced fade")
	}
	if _, err := c.RenderRGBA(false); err != nil {
		t.Fatal(err)
	}
	_ = c.Handle(Actions{Text: []rune{'N'}})
	for i := 0; i < 100; i++ {
		if c.fade != nil {
			_ = c.Handle(Actions{Cancel: true, Selector1: true, Selector2: true})
		}
		_ = c.Tick()
	}
	if c.Screen != ScreenBriefing || c.fade != nil || s != c.Mission || hash != s.Data.ObjectHash || draws != len(s.Data.Draws) {
		t.Fatal("fade altered mission or did not finish")
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	if c.fade == nil || c.fade.From != nil {
		t.Fatal("office entry fade")
	}
}
