package sounds

import (
	assetdata "gunship2000-assets/game/assets"
	"testing"
)

func TestIndependentMusicEffectsAndSpeechOutput(t *testing.T) {
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{"office_loop": {Samples: []int16{1000, 1000, 1000}, SampleRate: 49716}, "sfx": {Samples: []int16{200, 200, 200}, SampleRate: 49716}}, GS2Commands: map[uint16]assetdata.AudioGS2Command{10: {EventID: 10, TrackKey: "sfx"}}, GS2Digital: map[uint16]assetdata.AudioDigitalPlayback{0x2d: {Clip: &assetdata.AudioClip{Samples: []int16{800, 800}, SampleRate: 8000}, Priority: 1}}}
	for _, muted := range []bool{false, true} {
		p, e := NewAudioPlayer(a, AudioPreferences{MusicMode: MusicAll, MusicVolume: .37, EffectsVolume: .5, SpeechMuted: muted})
		if e != nil {
			t.Fatal(e)
		}
		p.Sync("office", nil, false)
		p.PlayGS2Event(10, 0)
		p.PlayGS2Event(0x2d, 0)
		out := make([]float32, 1)
		p.Render(out, 1)
		want := float32((370. + 100. + 400.) / 32768.)
		if muted {
			want = float32((370. + 100.) / 32768.)
		}
		if out[0] != want {
			t.Fatal(muted, out, want)
		}
		if p.digitalFrames != 1 {
			t.Fatal("muted voice clock reset/failed toadvance")
		}
		status := p.Status()
		if status.MusicVolume != .37 || status.EffectsVolume != .5 || status.SpeechEnabled == muted {
			t.Fatal(status)
		}
		p.SetMusicEnabled(false)
		p.SetSoundEffectsEnabled(false)
		p.Render(out, 1)
		if out[0] != 0 {
			t.Fatal("channelmute leaked", out)
		}
	}
}
