package assets

import (
	"fmt"
	"testing"
)

func TestHistoricalAudioFamiliesDoNotAcquireDefenseConstructors(t *testing.T) {
	for _, tc := range []struct{ schema, count int }{{5, 14}, {6, 16}, {7, 18}} {
		events := expectedGS2GameplayEvents(tc.schema)
		if len(events) != tc.count {
			t.Fatal(tc, events)
		}
		seen := map[int]bool{}
		for _, id := range events {
			if seen[id] {
				t.Fatal("duplicate", id)
			}
			seen[id] = true
		}
		if seen[0x0a] != (tc.schema >= 7) || seen[0x2a] != (tc.schema >= 7) || seen[4] != (tc.schema >= 6) || seen[5] != (tc.schema >= 6) {
			t.Fatal("historical family widened", tc, events)
		}
	}
}

func TestDefenseCapabilityRequiresExactCurrentConstructors(t *testing.T) {
	a := &AudioAssets{Clips: map[string]*AudioClip{}, GS2Commands: map[uint16]AudioGS2Command{}, ASoundGS2Data: make([]byte, 0x1000)}
	if a.SupportsDefenseReleaseAudio() || (*AudioAssets)(nil).SupportsDefenseReleaseAudio() {
		t.Fatal("empty historical audio claimed current capability")
	}
	for _, tc := range []struct {
		id                  uint16
		row, start, handler int
		raw                 []byte
	}{{0x0a, 2, 0x924, 0x93a, []byte{0x8d, 0x0e, 0x24, 0x09, 0xe9, 0xcf, 0xfd}}, {0x2a, 0, 0x99a, 0x933, []byte{0x8d, 0x0e, 0x9a, 0x09, 0xe9, 0xc8, 0xfd}}} {
		key := fmt.Sprintf("gs2_%02x", tc.id)
		a.GS2Commands[tc.id] = AudioGS2Command{EventID: tc.id, Channel: tc.row, Start: tc.start, HandlerCS: tc.handler, HandlerFile: tc.handler + 512, HandlerHex: fmt.Sprintf("%x", tc.raw), HandlerSHA256: Hash(tc.raw), TrackKey: key}
		a.Clips[key] = &AudioClip{Samples: []int16{1}, SampleRate: AudioSampleRate}
	}
	if !a.SupportsDefenseReleaseAudio() {
		t.Fatal("exact typed constructors rejected")
	}
	c := a.GS2Commands[0x0a]
	c.Start++
	a.GS2Commands[0x0a] = c
	if a.SupportsDefenseReleaseAudio() {
		t.Fatal("wrong stream accepted")
	}
	c.Start--
	a.GS2Commands[0x0a] = c
	a.Clips[c.TrackKey].LoopEndFrame = 1
	if a.SupportsDefenseReleaseAudio() {
		t.Fatal("sustained substitute accepted")
	}
}
