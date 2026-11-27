//go:build !headless

package menus

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/sounds"
	"os"
	"testing"
	"time"
)

func TestCompletedMissionFadeDoesNotDarkenNextLoadPrompt(t *testing.T) {
	s := pointWindow{fadeIn: true, transition: time.Now().Add(-time.Second)}
	s.input(nil, nil, false, false, &graphicsWindow{})
	s.open, s.loading = true, true
	if s.alpha() != 0 {
		t.Fatal("a completed fade obscured the next load prompt")
	}
}

func TestRestoredPointProducesRotorPCM(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("prepared sound data required")
	}
	a, err := assetdata.LoadAudioAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Supply only the real retained GS2 driver. Music, digital effects and
	// initialization WAVs cannot satisfy this test.
	p, err := sounds.NewAudioPlayer(&assetdata.AudioAssets{ASoundGS2Data: a.ASoundGS2Data}, sounds.DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	p.PlayInitializationStart()
	pcm := make([]float32, 512)
	audible := func() {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			p.Render(pcm, len(pcm))
			for _, v := range pcm {
				if v > 0.001 || v < -0.001 {
					return
				}
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("no audible rotor PCM", p.Status())
	}
	audible()
	for i := 0; i < 2; i++ {
		point := pointWindow{restored: true}
		point.syncRestoredAudio(p, 1) // Running at idle, including low collective.
		audible()
		point.syncRestoredAudio(p, 1)
		audible()
	}
	// A stopped bookmark must have a driver for future starts without replaying
	// the initialization cue or residual samples from the previous branch.
	point := pointWindow{restored: true}
	point.syncRestoredAudio(p, 0)
	for i := 0; i < 30; i++ {
		time.Sleep(time.Millisecond)
		p.Render(pcm, len(pcm))
		for _, v := range pcm {
			if v > .001 || v < -0.001 {
				t.Fatal("stopped bookmark replayed sound", v)
			}
		}
	}
	p.UpdateRotor(1)
	audible()

}

func TestMissionPointBusyPanelPrecedesWork(t *testing.T) {
	for _, load := range []bool{false, true} {
		s := pointWindow{open: true, loading: load}
		g := &graphicsWindow{}
		// A nil controller makes an accidental reconstruction unsafe. Both the
		// confirm frame and subsequent unpresented frames must stop before work.
		blocked, notice := s.input(nil, []int32{rl.KeyEnter}, false, false, g)
		if !blocked || notice != "" || !s.busy || s.presented || s.alpha() != 0 {
			t.Fatal(s, notice)
		}
		blocked, notice = s.input(nil, nil, false, false, g)
		if !blocked || notice != "" {
			t.Fatal("background work before showing busy panel", s, notice)
		}
		s.markPresented()
		if !s.presented {
			t.Fatal("presentation acknowledgement lost")
		}
		blocked, notice = s.input(nil, nil, false, false, g)
		if !blocked || notice == "" || s.open {
			t.Fatal("failed operation did not return cleanly", s, notice)
		}
	}
}

func TestSessionPointChordCannotIssueLand(t *testing.T) {
	for _, tc := range []struct {
		key              int32
		alt, point, land bool
	}{
		{rl.KeyL, true, true, false}, {rl.KeyK, true, false, false}, {rl.KeyL, false, false, true},
	} {
		var g graphicsWindow
		keys, point := g.takePointShortcut([]int32{tc.key}, tc.alt)
		held := func(k int32) bool { return !g.consumed[k] && k == tc.key }
		gps := sampleGPSCommandKey(held)
		direct := sampleWingmanOrderKey(held)
		if point != tc.point || (gps == "l") != tc.land || (direct == "l") != tc.land || (point && len(keys) != 0) {
			t.Fatalf("key=%d alt=%t point=%t keys=%v gps=%q direct=%q", tc.key, tc.alt, point, keys, gps, direct)
		}
	}
}
