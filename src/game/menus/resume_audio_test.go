package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	sound "gunship2000-assets/game/sounds"
	"os"
	"testing"
)

// Production launch/film/Resume plus the real prepared retained driver. This is
// a transport assertion, not a substitute for connected listening acceptance.
func TestResumeBeforeTakeoffRetainsFlightAudioOwner(t *testing.T) {
	c, f := viewerFilm(t)
	a, err := assetdata.LoadAudioAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	player, err := sound.NewAudioPlayer(a, sound.DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer player.Stop()
	q, err := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	c.Viewer = NewReplayViewer(q)
	c.Screen = ScreenReplay
	player.PlayInitializationStart()
	syncFrontEndAudio(player, c)
	if player.Status().Rotor == nil {
		t.Fatal("viewer has no retained GS2 driver")
	}
	prior := q.Preview().FilmCheckpoint()
	if err = c.activateReplayControl(1); err != nil {
		t.Fatal(err)
	}
	syncFrontEndAudio(player, c)
	if err = c.activateReplayControl(16); err != nil {
		t.Fatal(err)
	}
	syncFrontEndAudio(player, c)
	if player.Status().Rotor == nil {
		t.Fatal("Resume destroyed retained GS2 worker: later source0b commands are silently lost")
	}
	if compareFilmCheckpoint(prior, q.Preview().FilmCheckpoint()) != "" {
		t.Fatal("transport handoff altered logical state")
	}
	if q.Engine.Store != nil {
		t.Fatal("practice acquired career capability")
	}
	before := player.Status().InitializationCommands
	if err = q.HandlePractice(Actions{FlightCommands: []int{2}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		if err = q.AdvancePractice(.1); err != nil {
			t.Fatal(err)
		}
		syncFrontEndAudio(player, c)
	}
	if player.Status().Rotor == nil || player.Status().InitializationCommands != before {
		t.Fatal("practice lost or reinitialized driver", player.Status())
	}
}
func TestColdFilmEntryCreatesFlightAudioOwner(t *testing.T) {
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
	q, e := NewFilmPlayback(c.Office.Assets, f, c.Runtime)
	if e != nil {
		t.Fatal(e)
	}
	c.Screen = ScreenReplay
	c.Viewer = NewReplayViewer(q)
	syncFrontEndAudio(player, c)
	if player.Status().Rotor == nil {
		t.Fatal("cold film entry never constructed its GS2 audio driver")
	}
}
