package menus

import "testing"

func TestReplayAudioHandoffDiscardsMenuTails(t *testing.T) {
	player, _ := audioTestPlayer(t)
	child := &FrontEnd{Screen: ScreenScene}
	c := &FrontEnd{Screen: ScreenReplay, Viewer: &ReplayViewer{Playback: &FilmPlayback{Engine: child}}}
	c.emitAudioCue(gs4Accepted)
	syncFrontEndAudio(player, c)
	if len(c.takeAudioCues()) != 0 || player.Status().MusicOwner != "silent" {
		t.Fatal("PLAY retained a delayed menu sound", player.Status())
	}
	c.Viewer.Stopped = true
	syncFrontEndAudio(player, c)
	c.Viewer.Stopped = false
	syncFrontEndAudio(player, c)
	if child.Store != nil {
		t.Fatal("audio transport acquired career capability")
	}
}
