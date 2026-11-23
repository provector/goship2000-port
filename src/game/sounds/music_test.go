package sounds

import (
	"gunship2000-assets/game/assets"
	"os"
	"slices"
	"testing"
	"time"
)

func TestPackedMusicMixer(t *testing.T) {
	var players [2]*AudioPlayer
	for i, key := range []string{"GS_T2_LOOSE", "GS_T2_PACKED"} {
		root := os.Getenv(key)
		if root == "" {
			t.Skip("requires owned matching preparations")
		}
		a, e := assets.LoadAudioAssets(root)
		if e != nil {
			t.Fatal(e)
		}
		p, e := NewAudioPlayer(a, DefaultAudioPreferences())
		if e != nil {
			t.Fatal(e)
		}
		defer p.Stop()
		players[i] = p
		p.Sync("office", nil, false)
		p.PlayCue(0x0d)
	}
	if players[1].intro.Music == nil || players[1].effect == nil {
		t.Fatal("missing active stream or short effect")
	}
	var elapsed [2]time.Duration
	compare := func(blocks int) {
		var data [2][512]float32
		for n := 0; n < blocks; n++ {
			for i, p := range players {
				start := time.Now()
				p.Render(data[i][:], 512)
				elapsed[i] += time.Since(start)
			}
			if !slices.Equal(data[0][:], data[1][:]) {
				t.Fatalf("mixed PCM differs at %d", players[1].musicFrames)
			}
			if n%8 == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}
	compare(420) // More than six decoded pages, including worker-supplied pages.
	for _, p := range players {
		p.musicFrames = uint64(p.intro.Frames() + p.loop.Frames() - 100)
		p.PlayCue(0x0d)
	}
	// The last page is already queued during ordinary sequential playback. For
	// this supplied boundary preload that single page in the test worker.
	p := players[1]
	clip := p.loop.Music
	last := (clip.Frames - 1) / assets.MusicPageFrames
	if clip.Warm(last) == nil {
		p.music.request(clip, last)
		until := time.Now().Add(time.Second)
		for {
			page := p.music.pages[last%4].Load()
			if page != nil && page.clip == clip && page.number == last {
				break
			}
			if time.Now().After(until) {
				t.Fatal("loop end decode")
			}
			time.Sleep(time.Millisecond)
		}
	}
	compare(20)
	if misses := p.music.misses.Load(); misses != 0 {
		t.Fatalf("%d missed samples", misses)
	}
	t.Logf("440 x 512 mixed frames, callback elapsed loose=%v packed=%v, exact samples and zero missing", elapsed[0], elapsed[1])
}
