package sounds

import (
	"bytes"
	"errors"
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

type rotorPCM struct {
	read, pausedCalls, closes int
	paused                    bool
	commands                  []uint16
	failure                   string
}

func (r *rotorPCM) Command(v uint16) error { r.commands = append(r.commands, v); return nil }
func (r *rotorPCM) Read(dst []float32) {
	clear(dst)
	if r.paused {
		return
	}
	for i := range dst {
		dst[i] = .5
		r.read++
	}
}
func (r *rotorPCM) SetPaused(v bool) { r.paused = v; r.pausedCalls++ }
func (r *rotorPCM) Close()           { r.closes++ }
func (r *rotorPCM) Status() RotorStatus {
	return RotorStatus{Policy: RotorPolicy, Frames: uint64(r.read), Commands: uint64(len(r.commands) + 1), Failure: r.failure}
}

func TestRotorMixerMuteGainPauseAndOwnership(t *testing.T) {
	late := &assetdata.AudioClip{Samples: []int16{4000, 5000}, SampleRate: 49716}
	early := &assetdata.AudioClip{Samples: []int16{1000, 2000}, LoopStartFrame: 1, LoopEndFrame: 2, SampleRate: 49716}
	data := []byte{1, 2, 3}
	p, err := NewAudioPlayer(&assetdata.AudioAssets{ASoundGS2Data: data, Clips: map[string]*assetdata.AudioClip{InitializationStartClip: late, InitializationEarlyClip: early}}, AudioPreferences{MusicMode: MusicAll, MusicVolume: 1, EffectsVolume: .25})
	if err != nil {
		t.Fatal(err)
	}
	r := &rotorPCM{}
	opens := 0
	p.openRotor = func(got []byte) (rotorPlayback, error) {
		opens++
		if !bytes.Equal(got, data) {
			t.Fatal("wrong prepared initialized carrier")
		}
		return r, nil
	}
	p.Sync("silent", nil, false)
	p.PlayInitializationEarly()
	p.SetSoundEffectsEnabled(false)
	p.PlayInitializationStart()
	if p.effect != nil || opens != 1 || p.Status().InitializationCommands != 1 || p.Status().MusicOwner != "silent" {
		t.Fatal("continuous late owner failed to replace early owner while muted", p.Status())
	}
	out := make([]float32, 2)
	p.Render(out, 2)
	if out[0] != 0 || r.read != 2 {
		t.Fatal("mute should suppress output while consuming current rotor PCM", out, r.read)
	}
	p.SetSoundEffectsEnabled(true)
	p.Render(out, 2)
	if out[0] != .125 || out[1] != .125 || opens != 1 || r.read != 4 {
		t.Fatal("unmute restarted owner or wrong independent effects gain", out, opens, r.read)
	}
	p.UpdateRotor(15)
	p.UpdateRotor(0)
	if len(r.commands) != 2 || r.commands[0] != 15 || r.commands[1] != 0 {
		t.Fatal("source arguments reordered or filtered", r.commands)
	}
	p.SetEffectsPaused(true)
	p.Render(out, 2)
	if out[0] != 0 || r.read != 4 || !r.paused {
		t.Fatal("pause advanced retained PCM cursor", out, r.read)
	}
	p.SetEffectsPaused(false)
	p.prefs.EffectsVolume = 0
	p.Render(out, 2)
	if out[0] != 0 || r.read != 6 {
		t.Fatal("volume zero changed rotor clock")
	}
	p.SetMusicEnabled(false)
	p.SetMusicEnabled(true)
	if opens != 1 || p.Status().MusicOwner != "silent" {
		t.Fatal("music preference changed flight sound/owner")
	}
	p.StopRotor()
	p.StopRotor()
	if r.closes != 1 || p.Status().Rotor != nil {
		t.Fatal("leaving scene failed to release worker once")
	}
	p.Sync("office", nil, false)
	p.PlayInitializationStart()
	p.Stop()
	if r.closes != 2 || p.Status().Rotor != nil {
		t.Fatal("reentry/confirmed quit failed rotor cleanup")
	}
}

func TestRotorFailureIsVisibleAndBoundedFallback(t *testing.T) {
	clip := &assetdata.AudioClip{Samples: []int16{4000, 5000}, SampleRate: 49716}
	p, err := NewAudioPlayer(&assetdata.AudioAssets{ASoundGS2Data: []byte{1}, Clips: map[string]*assetdata.AudioClip{InitializationStartClip: clip}}, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	p.openRotor = func([]byte) (rotorPlayback, error) { return nil, errors.New("test missing packaged helper") }
	p.PlayInitializationStart()
	out := make([]float32, 3)
	p.Render(out, 3)
	if out[0] != 4000.0/32768 || out[1] != 5000.0/32768 || out[2] != 0 || p.Status().RotorFailure == "" || p.rotorFailureLogged == "" {
		t.Fatal("missing helper must report and preserve bounded accepted cue", out, p.Status())
	}
	// A stopped worker is reported on the application Sync thread. Neither
	// Render nor Sync attempts an unbounded helper restart or replays startup.
	r := &rotorPCM{failure: "test broken pipe"}
	p.openRotor = func([]byte) (rotorPlayback, error) { return r, nil }
	p.PlayInitializationStart()
	p.Render(out, 3)
	if out[0] != 0 || out[1] != 0 || out[2] != 0 {
		t.Fatal("worker failure replayed startup in flight", out)
	}
	p.Sync("silent", nil, false)
	logged := p.rotorFailureLogged
	p.Render(out, 3)
	p.Sync("silent", nil, false)
	if out[0] != 0 || logged != "test broken pipe" || p.rotorFailureLogged != logged || r.read != 0 {
		t.Fatal("failed worker did not remain silent", out, p.Status())
	}
	p.Stop()
}

func TestMissingDeviceNeverStartsOrQueuesRotorHelper(t *testing.T) {
	p, err := NewAudioPlayer(&assetdata.AudioAssets{ASoundGS2Data: []byte{1}, Clips: map[string]*assetdata.AudioClip{InitializationStartClip: {Samples: []int16{4000}}}}, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	opens := 0
	p.openRotor = func([]byte) (rotorPlayback, error) { opens++; return &rotorPCM{}, nil }
	p.SetDeviceClock(false) // Explicit device failure; default headless tests differ.
	p.PlayInitializationStart()
	for i := 0; i < 100; i++ {
		p.UpdateRotor(uint16(i & 15))
	}
	if opens != 0 || p.Status().Rotor != nil || p.Status().RotorFailure == "" || p.Status().InitializationCommands != 1 {
		t.Fatal("missing device started a helper or lost the once-only event", opens, p.Status())
	}
	pcm := make([]float32, 2)
	p.Render(pcm, 2)
	if pcm[0] != 4000.0/32768 || pcm[1] != 0 {
		t.Fatal("missing-device policy lost the bounded initialization carrier", pcm)
	}
	p.SetDeviceClock(true)
	p.PlayInitializationStart()
	if opens != 1 {
		t.Fatal("future scene after restored device could not start rotor")
	}
	r := p.rotor.(*rotorPCM)
	p.SetDeviceClock(false)
	if r.closes != 1 || p.Status().Rotor != nil {
		t.Fatal("device loss did not release current helper")
	}
}
