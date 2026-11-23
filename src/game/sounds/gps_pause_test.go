package sounds

import (
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestGPSPauseRetainsSampleCursorNotMute(t *testing.T) {
	p, e := NewAudioPlayer(&assetdata.AudioAssets{}, DefaultAudioPreferences())
	if e != nil {
		t.Fatal(e)
	}
	p.effect = &assetdata.AudioClip{Samples: []int16{8192, 16384, 24576}}
	v := make([]float32, 1)
	p.Render(v, 1)
	if p.effectFrames != 1 || v[0] != .25 {
		t.Fatal(v, p.effectFrames)
	}
	p.SetEffectsPaused(true)
	p.Render(v, 1)
	if v[0] != 0 || p.effectFrames != 1 {
		t.Fatal("pausedconsumedframe")
	}
	p.SetEffectsPaused(false)
	p.Render(v, 1)
	if v[0] != .5 || p.effectFrames != 2 {
		t.Fatal("resumereplayedcue")
	}
	p.SetSoundEffectsEnabled(false)
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(true)
	p.Render(v, 1)
	if v[0] != 0 {
		t.Fatal("mute resurrectedeffect")
	}
}
