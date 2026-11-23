package menus

import (
	assetdata "gunship2000-assets/game/assets"
	sound "gunship2000-assets/game/sounds"
	"testing"
)

func TestInitializationAudioOnceAcrossSceneAndModals(t *testing.T) {
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{sound.InitializationStartClip: {Samples: []int16{1000, 2000, 3000}, SampleRate: 49716}}}
	p, err := sound.NewAudioPlayer(a, sound.AudioPreferences{MusicMode: sound.MusicAll, MusicVolume: 1, EffectsVolume: .5})
	if err != nil {
		t.Fatal(err)
	}
	c := &FrontEnd{Screen: ScreenSceneInitialization, SceneEntry: &sceneInitialization{}}
	syncFrontEndAudio(p, c)
	if p.Status().InitializationCommands != 0 {
		t.Fatal("unprepared initialization sounded")
	}
	c.SceneEntry.soundPending = true
	syncFrontEndAudio(p, c)
	if c.SceneEntry.soundPending || p.Status().InitializationCommands != 1 || p.Status().MusicOwner != "silent" {
		t.Fatal("ready one-shot/mute ownership", p.Status())
	}
	c.Screen = ScreenQuit
	c.previousScreen = ScreenSceneInitialization
	syncFrontEndAudio(p, c)
	c.Screen = ScreenSceneInitialization
	syncFrontEndAudio(p, c)
	c.Screen = ScreenScene
	c.SceneEntry = nil
	syncFrontEndAudio(p, c)
	c.Screen = ScreenQuit
	c.previousScreen = ScreenScene
	syncFrontEndAudio(p, c)
	p.SetMusicEnabled(false)
	p.SetMusicEnabled(true)
	syncFrontEndAudio(p, c)
	if p.Status().InitializationCommands != 1 || p.Status().MusicOwner != "silent" {
		t.Fatal("modal/re-enable replayed cue", p.Status())
	}
	data := make([]float32, 1)
	p.Render(data, 1)
	if data[0] != 0 {
		t.Fatal("scene modal must suspend effects without consuming their cursor", data)
	}
	c.Screen = ScreenScene
	syncFrontEndAudio(p, c)
	p.Render(data, 1)
	if data[0] != 500.0/32768 {
		t.Fatal("scene entry cut bounded startup tail or lost gain", data)
	}
	// The consumed event must stay consumed while effects are disabled.
	p.SetSoundEffectsEnabled(false)
	c.Screen = ScreenSceneInitialization
	c.SceneEntry = &sceneInitialization{soundPending: true}
	syncFrontEndAudio(p, c)
	if c.SceneEntry.soundPending {
		t.Fatal("muted event retained for replay")
	}
	p.SetSoundEffectsEnabled(true)
	syncFrontEndAudio(p, c)
	if p.Status().InitializationCommands != 1 {
		t.Fatal("unmute replayed initialization")
	}
	c.Screen = ScreenBriefing
	syncFrontEndAudio(p, c)
	if p.Status().MusicOwner != "office" {
		t.Fatal("briefing did not restore menu owner")
	}
	c.Screen = ScreenSceneInitialization
	c.SceneEntry = &sceneInitialization{soundPending: true}
	syncFrontEndAudio(p, c)
	if p.Status().InitializationCommands != 2 {
		t.Fatal("second real entry missing sound")
	}
	c.Screen = ScreenQuit
	c.previousScreen = ScreenSceneInitialization
	c.ExitRequested = true
	syncFrontEndAudio(p, c)
	if p.Status().MusicOwner != "" {
		t.Fatal("quit did not close mixer")
	}
}
