package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	sound "gunship2000-assets/game/sounds"
	"os"
	"testing"
)

// A non-nil worker alone does not establish a faithful takeover: replacing it
// with a freshly initialized worker would also be non-nil and lose its history.
// This protects the source40/41 transport boundary against another19h init.
func TestResumeRetainsInitializationAcrossTransportModes(t *testing.T) {
	for _, mode := range []string{"playing", "stop-play", "demo", "rewind"} {
		t.Run(mode, func(t *testing.T) {
			c, f := viewerFilm(t)
			a, e := assetdata.LoadAudioAssets(os.Getenv("GS2000_TEST_MISSIONS"))
			if e != nil {
				t.Fatal(e)
			}
			player, e := sound.NewAudioPlayer(a, sound.DefaultAudioPreferences())
			if e != nil {
				t.Fatal(e)
			}
			defer player.Stop()
			q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
			if e != nil {
				t.Fatal(e)
			}
			c.Viewer = NewReplayViewer(q)
			c.Screen = ScreenReplay
			syncFrontEndAudio(player, c)
			for i := 0; i < 8; i++ {
				if e = q.Step(); e != nil {
					t.Fatal(e)
				}
				syncFrontEndAudio(player, c)
			}
			if mode == "stop-play" {
				if e = c.activateReplayControl(1); e != nil {
					t.Fatal(e)
				}
				syncFrontEndAudio(player, c)
				if e = c.activateReplayControl(2); e != nil {
					t.Fatal(e)
				}
				syncFrontEndAudio(player, c)
			}
			if mode == "demo" {
				if e = c.activateReplayControl(17); e != nil {
					t.Fatal(e)
				}
				syncFrontEndAudio(player, c)
			}
			if mode == "rewind" {
				before := player.Status().InitializationCommands
				if e = c.activateReplayControl(0); e != nil {
					t.Fatal(e)
				}
				syncFrontEndAudio(player, c)
				if player.Status().InitializationCommands != before+1 {
					t.Fatal("successful rewind did not initialize exactly one new sound session")
				}
			}
			before := player.Status().InitializationCommands
			checkpoint := c.Viewer.Playback.Preview().FilmCheckpoint()
			if e = c.activateReplayControl(16); e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 3; i++ {
				syncFrontEndAudio(player, c)
			}
			if player.Status().Rotor == nil || player.Status().InitializationCommands != before {
				t.Fatal("Resume/redraw reinitialized retained sound history", mode, player.Status())
			}
			if compareFilmCheckpoint(checkpoint, c.Viewer.Playback.Preview().FilmCheckpoint()) != "" {
				t.Fatal("audio takeover mutated authoritative film state")
			}
			if c.Viewer.Playback.Engine.Store != nil {
				t.Fatal("audio takeover acquired career writer")
			}
		})
	}
}
