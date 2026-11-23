package menus

import (
	assetdata "gunship2000-assets/game/assets"
	sound "gunship2000-assets/game/sounds"
	"testing"
)

func audioTestAssets() *assetdata.AudioAssets {
	clip := func(xs ...int16) *assetdata.AudioClip {
		return &assetdata.AudioClip{Samples: xs, SampleRate: assetdata.AudioSampleRate, BusyEndFrame: uint64(len(xs))}
	}
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{"logo": clip(100, 200, 0), "title": clip(300, 400, 0, 0), "office_intro": clip(1000, 2000), "office_loop": clip(3000, 4000), "gs4_0d": clip(600, 700), "gs4_22": clip(100, 200, 0)}}
	a.Clips["title"].BusyEndFrame = 2
	return a
}

func audioTestPlayer(t *testing.T) (*sound.AudioPlayer, *FrontEnd) {
	t.Helper()
	p, e := sound.NewAudioPlayer(audioTestAssets(), sound.DefaultAudioPreferences())
	if e != nil {
		t.Fatal(e)
	}
	return p, &FrontEnd{Screen: ScreenOffice}
}

func TestAudioSampleClockIntroLoopAndMenuContinuity(t *testing.T) {
	p, c := audioTestPlayer(t)
	syncFrontEndAudio(p, c)
	p.SetDeviceClock(true)
	data := make([]float32, 7)
	p.Render(data[:3], 3)
	p.Render(data[3:], 4)
	want := []int16{1000, 2000, 3000, 4000, 3000, 4000, 3000}
	for i, v := range want {
		if data[i] != float32(v)/32768 {
			t.Fatalf("gap/order at%d: %v", i, data)
		}
	}
	for _, s := range []Screen{ScreenRoster, ScreenMission, ScreenBriefing, ScreenPapers, ScreenSettings, ScreenOrdnance, ScreenOutfit, ScreenOffice} {
		c.Screen = s
		syncFrontEndAudio(p, c)
	}
	if p.Status().MusicFrames != 7 {
		t.Fatal("screen change restarted music")
	}
	c.previousScreen = c.Screen
	c.Screen = ScreenQuit
	syncFrontEndAudio(p, c)
	p.Render(data[:1], 1)
	c.Screen = ScreenOffice
	syncFrontEndAudio(p, c)
	if p.Status().MusicFrames != 8 {
		t.Fatal("quit cancel restarted/paused music")
	}
	c.ExitRequested = true
	syncFrontEndAudio(p, c)
	p.Render(data, 7)
	for _, v := range data {
		if v != 0 {
			t.Fatal("quit left active playback")
		}
	}
}

func TestAudioIndependentEnableSwitchesAndReplacement(t *testing.T) {
	p, c := audioTestPlayer(t)
	syncFrontEndAudio(p, c)
	p.PlayCue(0x22)
	data := make([]float32, 1)
	p.Render(data, 1)
	if data[0] != 1100.0/32768 {
		t.Fatal("music/effect mix", data)
	}
	p.SetSoundEffectsEnabled(false)
	p.Render(data, 1)
	if data[0] != 2000.0/32768 {
		t.Fatal("disabled effects still audible")
	}
	p.PlayCue(0x13)
	if p.Status().EffectCommands != 1 {
		t.Fatal("disabled effect queued")
	}
	p.SetSoundEffectsEnabled(true)
	if p.Status().MusicFrames != 2 {
		t.Fatal("enabling effect restarted music")
	}
	p.SetMusicEnabled(false)
	p.PlayCue(0x13)
	p.Render(data, 1)
	if data[0] != 600.0/32768 {
		t.Fatal("music disable affected enabled effect")
	}
	p.SetMusicEnabled(true)
	p.Render(data, 1)
	if data[0] != 1700.0/32768 {
		t.Fatal("music reenable/effect progress", data)
	}
	p.PlayCue(0x22)
	p.Render(data, 1)
	if data[0] != 2100.0/32768 {
		t.Fatal("effect did not replace old owner")
	}
	if p.Status().LastCue != 0x22 {
		t.Fatal("source cue identity lost")
	}
}

func TestAudioMusicEnableDoesNotReplayFinishedStartup(t *testing.T) {
	p, c := audioTestPlayer(t)
	c.Screen = ScreenStartup
	c.Startup = &StartupPlayback{Stage: 0}
	syncFrontEndAudio(p, c)
	p.SetMusicEnabled(false)
	c.Startup.Stage = 1
	syncFrontEndAudio(p, c)
	c.Startup.Done = true
	c.Screen = ScreenOffice
	syncFrontEndAudio(p, c)
	p.SetMusicEnabled(true)
	if p.Status().MusicOwner != "office" {
		t.Fatal("replayed finished startup")
	}
	out := make([]float32, 1)
	p.Render(out, 1)
	if out[0] != 1000.0/32768 {
		t.Fatal("wrong reenable owner")
	}
}

func TestAudioTitleBusyWaitUsesLogicalEndNotTail(t *testing.T) {
	a := testStartupAssets()
	a.Stages = a.Stages[1:]
	a.Stages[0].Frames[0].HoldSeconds = 0
	p, e := NewStartupPlayback(a)
	if e != nil {
		t.Fatal(e)
	}
	busy := true
	p.AudioBusy = func(int) bool { return busy }
	p.Advance(.1)
	if p.Done || !p.waitingAudio {
		t.Fatal("visual completion ignored source busy")
	}
	p.Advance(.1)
	if p.Done {
		t.Fatal("busy wait advanced again")
	}
	busy = false
	p.Advance(0)
	if !p.Done {
		t.Fatal("logical end did not release visual stage")
	}
}

func TestAudioNativeQuitConfirmationWithoutTextEvent(t *testing.T) {
	c := sourceFrontEnd(t, true)
	input := &windowInput{}
	viewport := officeViewport(960, 720, false)
	step := func(b Buttons) {
		t.Helper()
		if err := c.Handle(input.ReadViewport(b, viewport)); err != nil {
			t.Fatal(err)
		}
	}
	step(Buttons{AltQ: true, AnyKeyPressed: true})
	if c.Screen != ScreenQuit {
		t.Fatal("quit modal not opened")
	}
	step(Buttons{})
	// A physical Y event can lack a text event, especially with modifiers/IME.
	// The generic any-key response must not consume it as a cancellation.
	step(Buttons{ConfirmQuit: true, AnyKeyPressed: true})
	if !c.ExitRequested {
		t.Fatal("physical Y did not confirm quit")
	}
	p, err := sound.NewAudioPlayer(audioTestAssets(), sound.DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	syncFrontEndAudio(p, c)
	out := make([]float32, 32)
	p.Render(out, 32)
	for _, sample := range out {
		if sample != 0 {
			t.Fatal("quit left audio playing")
		}
	}
}
