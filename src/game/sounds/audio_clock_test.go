package sounds

import (
	"bytes"
	"errors"
	assetdata "gunship2000-assets/game/assets"
	"strings"
	"testing"
	"time"
)

func TestAudioModesAndSilentDeviceClock(t *testing.T) {
	for _, mode := range []MusicMode{MusicAll, MusicBackgroundOff, MusicOff} {
		prefs := DefaultAudioPreferences()
		prefs.MusicMode = mode
		p, e := NewAudioPlayer(audioTestAssets(), prefs)
		if e != nil {
			t.Fatal(e)
		}
		now := time.Unix(1000, 0)
		p.now = func() time.Time { return now }

		p.Sync("title", nil, false)
		if p.MusicBusy() != (mode != MusicOff) {
			t.Fatal("source title music mode", mode)
		}
		log := &bytes.Buffer{}
		close := StartAudioOutput(p, func(*AudioPlayer) (func(), error) { return nil, errors.New("test device unavailable") }, log)
		if !strings.Contains(log.String(), "unavailable") {
			t.Fatal("missing actionable device warning")
		}
		now = now.Add(time.Second)
		if p.MusicBusy() {
			t.Fatal("silent device strands startup")
		}
		p.Sync("office", nil, false)
		out := make([]float32, 1)
		p.Render(out, 1)
		if (out[0] != 0) != (mode == MusicAll) {
			t.Fatal("background mode", mode)
		}
		p.PlayCue(0x13)
		p.Render(out, 1)
		if mode != MusicAll && out[0] != 600.0/32768 {
			t.Fatal("music mode muted menu effects")
		}
		close()
	}
}

func audioTestAssets() *assetdata.AudioAssets {
	clip := func(xs ...int16) *assetdata.AudioClip {
		return &assetdata.AudioClip{Samples: xs, SampleRate: assetdata.AudioSampleRate, BusyEndFrame: uint64(len(xs))}
	}
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{"logo": clip(100, 200, 0), "title": clip(300, 400, 0, 0), "office_intro": clip(1000, 2000), "office_loop": clip(3000, 4000), "gs4_0d": clip(600, 700), "gs4_22": clip(100, 200, 0)}}
	a.Clips["title"].BusyEndFrame = 2
	return a
}
