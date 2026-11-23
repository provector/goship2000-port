package sounds

import (
	"bytes"
	"encoding/hex"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestGS2AcquisitionDigitalOriginalWrappersAndPayloads(t *testing.T) {
	source := initializationSource(t)
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("set separate complete prepared audio tree")
	}
	a, err := assetdata.LoadAudioAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	bank, err := os.ReadFile(testfixture.Path("installation/TARGET.BIN"))
	if err != nil {
		t.Fatal(err)
	}
	// Literal original ASOUND table/constructor/descriptor operands establish
	// priority1, the phrase-class prefix and signed left/center/right suffix.
	// These are existing logical lists, not same-number GS4 menu effects.
	for _, q := range []struct {
		event                                          uint16
		handler, descriptor                            int
		wrapper, list                                  string
		prefixStart, prefixEnd, suffixStart, suffixEnd int
	}{
		{0x2d, 0xb50, 0xc4f, "b901008d1e4f0ce95702", "e20a01000000541ce30a00000000", 1, 2785, 2787, 7251},
		{0x2e, 0xb5a, 0xc5d, "b901008d1e5d0ce94d02", "e20a01000000ce3a7d2e00000000", 1, 2785, 11901, 15053},
		{0x30, 0xb6e, 0xc79, "b901008d1e790ce93902", "e20a010000003654af4800000000", 1, 2785, 18607, 21557},
		{0x45, 0xbf0, 0xd2f, "b901008d1e2f0de9b701", "63e99add0000541ce30a00000000", 56730, 59746, 2787, 7251},
		{0x46, 0xbfa, 0xd3d, "b901008d1e3d0de9ad01", "63e99add0000ce3a7d2e00000000", 56730, 59746, 11901, 15053},
		{0x48, 0xc0e, 0xd59, "b901008d1e590de99901", "63e99add00003654af4800000000", 56730, 59746, 18607, 21557},
	} {
		if audioWord(source, 512+0x104f+2*int(q.event)) != uint16(q.handler) {
			t.Fatal("original event table changed", q.event)
		}
		for _, v := range []struct {
			at  int
			raw string
		}{{q.handler, q.wrapper}, {q.descriptor, q.list}} {
			want, e := hex.DecodeString(v.raw)
			if e != nil || !bytes.Equal(source[512+v.at:][:len(want)], want) {
				t.Fatalf("event%02x original CS%04x", q.event, v.at)
			}
		}
		entry, ok := a.GS2Digital[q.event]
		if !ok || entry.Priority != 1 || entry.Clip.SampleRate != 8000 {
			t.Fatalf("event%02x admission/priority/clock", q.event)
		}
		want := append(append([]byte(nil), bank[q.prefixStart:q.prefixEnd]...), bank[q.suffixStart:q.suffixEnd]...)
		if len(entry.Clip.Samples) != len(want) {
			t.Fatal("exact logical block lengths", q.event)
		}
		for i, sample := range want {
			if entry.Clip.Samples[i] != int16((int(sample)-128)<<8) {
				t.Fatalf("event%02x source payload sample%d", q.event, i)
			}
		}
	}
}

func TestGS2AcquisitionDigitalQueueRetainsRotorAndPauseOwnership(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("set separate complete prepared audio tree")
	}
	a, err := assetdata.LoadAudioAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	r := &combatFakeRotor{}
	p.rotor = r
	for _, command := range []uint16{0x2d, 0x2e, 0x30, 0x45} {
		if err = p.PlayGS2Event(command, 0); err != nil {
			t.Fatal(err)
		}
	}
	// All acquisition lists have priority1. Active list2d continues; the first
	// two pending equals occupy their source slots, and another equal is dropped.
	if p.digitalCurrent.event != 0x2d || p.digitalPending[0].event != 0x2e || p.digitalPending[1].event != 0x30 {
		t.Fatal("equal-priority acquisition list ordering changed")
	}
	if len(r.fixed) != 0 || p.rotor != r {
		t.Fatal("digital acquisition reset/replaced the continuous rotor owner")
	}
	p.SetEffectsPaused(true)
	p.PlayGS2Event(0x34, 0)
	p.Render(make([]float32, 128), 128)
	if p.digitalCurrent.event != 0x2d || p.digitalFrames != 0 || p.digitalPending[0].event != 0x2e {
		t.Fatal("modal pause advanced or queued an old acquisition command")
	}
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(false)
	silence := make([]float32, 128)
	p.Render(silence, 128)
	if p.digitalFrames != 128 {
		t.Fatal("effects mute must retain ongoing speech cursor")
	}
	for _, x := range silence {
		if x != 0 {
			t.Fatal("muted acquisition leaked output")
		}
	}
	p.SetSoundEffectsEnabled(true)
	p.PlayGS2Event(0x34, 0)
	if p.digitalCurrent.event != 0x2d || p.digitalPending[0].event != 0x34 || p.digitalPending[1].event != 0x2e {
		t.Fatal("higher-priority ammunition list should reorder pending without preemption")
	}
	p.StopRotor()
	if p.digitalCurrent != nil || p.digitalPending != ([2]*gs2DigitalVoice{}) {
		t.Fatal("scene exit left acquisition speech queued")
	}
}
