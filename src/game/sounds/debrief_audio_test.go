package sounds

import (
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestDebriefMusicGateOwnerAndBusyBoundary(t *testing.T) {
	for _, mode := range []MusicMode{MusicAll, MusicBackgroundOff, MusicOff} {
		a := audioTestAssets()
		a.Clips["office_loop"] = &assetdata.AudioClip{Samples: []int16{6000, 7000}, LoopEndFrame: 2}
		a.Clips["gs4_1c"] = &assetdata.AudioClip{Samples: []int16{1000, 2000, 3000, 4000}, BusyEndFrame: 2}
		prefs := DefaultAudioPreferences()
		prefs.MusicMode = mode
		prefs.EffectsVolume = 0
		prefs.MusicVolume = .5
		p, e := NewAudioPlayer(a, prefs)
		if e != nil {
			t.Fatal(e)
		}
		p.SetSoundEffectsEnabled(false)
		p.Sync("debrief", []uint16{0x1c}, false)
		out := make([]float32, 3)
		p.Render(out, 3)
		if mode == MusicOff {
			if out[0] != 0 || out[1] != 0 || out[2] != 0 {
				t.Fatal("all music off leaked result cue")
			}
		} else {
			if out[0] != 500.0/32768 || out[1] != 1000.0/32768 {
				t.Fatal("GS4 result incorrectly used effects gate/volume", mode, out)
			}
			if mode == MusicAll && out[2] != 3000.0/32768 {
				t.Fatal("source busy43 boundary failed to resume canonical background", out)
			}
			if mode == MusicBackgroundOff && out[2] != 1500.0/32768 {
				t.Fatal("background-off should retain cue's bounded audible release", out)
			}
		}
		if s := p.Status(); s.DebriefCommands != 1 || s.LastCue != 0x1c || s.MusicOwner != "debrief" || s.EffectCommands != 0 {
			t.Fatal("debrief ownership", s)
		}
		p.Sync("debrief", nil, false)
		if p.Status().DebriefCommands != 1 {
			t.Fatal("ordinary redraw retriggered result cue")
		}
		p.SetMusicEnabled(false)
		p.Render(out, 3)
		for _, x := range out {
			if x != 0 {
				t.Fatal("global music disable leaked")
			}
		}
		p.SetMusicEnabled(true)
		p.Render(out, 3)
		for _, x := range out {
			if x != 0 {
				t.Fatal("unmute replayed consumed result cue")
			}
		}
		p.Sync("office", nil, false)
		p.Render(out, 1)
		if mode == MusicAll && out[0] != 500.0/32768 {
			t.Fatal("return to office failed normal intro")
		}
	}
}

func TestDebriefIndefiniteSourceLoopDoesNotResumeOffice(t *testing.T) {
	a := audioTestAssets()
	a.Clips["gs4_1f"] = &assetdata.AudioClip{Samples: []int16{10, 20, 30, 40}, LoopStartFrame: 2, LoopEndFrame: 4}
	p, e := NewAudioPlayer(a, DefaultAudioPreferences())
	if e != nil {
		t.Fatal(e)
	}
	p.Sync("debrief", []uint16{0x1f}, false)
	out := make([]float32, 8)
	p.Render(out, 8)
	for i, want := range []int16{10, 20, 30, 40, 30, 40, 30, 40} {
		if out[i] != float32(want)/32768 {
			t.Fatal("GS4 FD music cycle/background ownership", i, out)
		}
	}
	p.Sync("office", nil, false)
	p.Render(out, 1)
	if out[0] != 1000.0/32768 {
		t.Fatal("exit failed source music replacement")
	}
}
