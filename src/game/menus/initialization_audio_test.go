package menus

import (
	"testing"

	assetdata "gunship2000-assets/game/assets"
	sound "gunship2000-assets/game/sounds"
)

func TestEarlyInitializationEventReproduction(t *testing.T) {
	// Synthetic PCM isolates the semantic call site. Event identity/order is
	// independently checked against GS2:0323 and ASOUND.GS2:a34 in sounds tests.
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{
		"gs2_initialization_early": {Samples: []int16{1000, 2000, 3000}, SampleRate: 49716},
	}}
	p, err := sound.NewAudioPlayer(a, sound.DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	c := &FrontEnd{Screen: ScreenSceneInitialization, SceneEntry: &sceneInitialization{}}
	syncFrontEndAudio(p, c)
	if p.Status().EffectCommands != 1 {
		t.Fatal("GS2 arrival before loader messages did not issue source15h", p.Status())
	}
	data := make([]float32, 1)
	p.Render(data, 1)
	if data[0] != 1000.0/32768 {
		t.Fatal("early initialization event was suppressed", data)
	}
}

func TestInitializationAudioEarlyLateModalReentryAndFailure(t *testing.T) {
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{
		sound.InitializationEarlyClip: {Samples: []int16{1000, 2000, 3000}, SampleRate: 49716, LoopStartFrame: 1, LoopEndFrame: 3},
		sound.InitializationStartClip: {Samples: []int16{4000, 5000}, SampleRate: 49716},
	}}
	p, err := sound.NewAudioPlayer(a, sound.AudioPreferences{MusicMode: sound.MusicAll, MusicVolume: 1, EffectsVolume: .5})
	if err != nil {
		t.Fatal(err)
	}
	c := &FrontEnd{Screen: ScreenSceneInitialization, SceneEntry: &sceneInitialization{}}
	syncFrontEndAudio(p, c)
	c.Screen, c.previousScreen = ScreenQuit, ScreenSceneInitialization
	syncFrontEndAudio(p, c)
	c.Screen = ScreenSceneInitialization
	p.SetMusicEnabled(false)
	p.SetMusicEnabled(true)
	syncFrontEndAudio(p, c)
	if p.Status().EarlyInitializationCommands != 1 || p.Status().InitializationCommands != 0 || p.Status().MusicOwner != "silent" {
		t.Fatal("early modal/music ownership", p.Status())
	}
	c.SceneEntry.soundPending = true
	syncFrontEndAudio(p, c)
	c.SceneEntry, c.Screen = nil, ScreenScene
	syncFrontEndAudio(p, c)
	data := make([]float32, 1)
	p.Render(data, 1)
	if data[0] != 2000.0/32768 || p.Status().InitializationCommands != 1 || p.Status().EarlyInitializationCommands != 1 {
		t.Fatal("early not replaced once by later rotor", data, p.Status())
	}
	c.Screen, c.SceneEntry = ScreenSceneInitialization, &sceneInitialization{}
	syncFrontEndAudio(p, c)
	c.Screen, c.SceneEntry = ScreenSettings, nil // Native candidate failed after opening line.
	syncFrontEndAudio(p, c)
	p.Render(data, 1)
	if data[0] != 0 || p.Status().EarlyInitializationCommands != 2 || p.Status().MusicOwner != "office" {
		t.Fatal("failed initialization loop leaked into settings", data, p.Status())
	}
	p.SetSoundEffectsEnabled(false)
	c.Screen, c.SceneEntry = ScreenSceneInitialization, &sceneInitialization{}
	syncFrontEndAudio(p, c)
	p.SetSoundEffectsEnabled(true)
	syncFrontEndAudio(p, c)
	if p.Status().EarlyInitializationCommands != 2 || !c.SceneEntry.earlySoundIssued {
		t.Fatal("muted entry replayed on unmute", p.Status())
	}
}
