package assets

import (
	"fmt"
	"testing"
)

func TestGunAudioSchemaDoesNotWidenHistoricalFamilies(t *testing.T) {
	for _, tc := range []struct{ schema, count int }{{5, 14}, {6, 16}, {7, 18}, {8, 19}} {
		events := expectedGS2GameplayEvents(tc.schema)
		if len(events) != tc.count {
			t.Fatal(tc, events)
		}
		found := false
		for _, event := range events {
			found = found || event == 0x29
		}
		if found != (tc.schema == 8) {
			t.Fatal("historical gun command family widened", tc, events)
		}
	}
}

func TestGunAudioCapabilityRequiresExactDescriptorAndClip(t *testing.T) {
	if (*AudioAssets)(nil).SupportsStationaryHostileAudio() {
		t.Fatal("nil capability")
	}
	a := &AudioAssets{GS2Commands: map[uint16]AudioGS2Command{}, Clips: map[string]*AudioClip{}, ASoundGS2Data: make([]byte, 0x1000)}
	if a.SupportsStationaryHostileAudio() {
		t.Fatal("historical audio claimed gun29")
	}
	raw := []byte{0x8d, 0x0e, 0xe0, 0x08, 0xe9, 0xf2, 0xfd}
	c := AudioGS2Command{EventID: 0x29, Channel: 1, Start: 0x8e0, HandlerCS: 0x910, HandlerFile: 0xb10, HandlerHex: fmt.Sprintf("%x", raw), HandlerSHA256: Hash(raw), TrackKey: "gs2_29"}
	a.GS2Commands[0x29] = c
	a.Clips[c.TrackKey] = &AudioClip{SampleRate: AudioSampleRate, Samples: []int16{1}}
	if !a.SupportsStationaryHostileAudio() {
		t.Fatal("exact source constructor rejected")
	}
	c.Start++
	a.GS2Commands[0x29] = c
	if a.SupportsStationaryHostileAudio() {
		t.Fatal("wrong source stream accepted")
	}
	c.Start--
	a.GS2Commands[0x29] = c
	a.Clips[c.TrackKey].LoopEndFrame = 1
	if a.SupportsStationaryHostileAudio() {
		t.Fatal("sustained substitute accepted")
	}
}
