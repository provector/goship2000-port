package sounds

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

func TestEarlyAudioSourceAndHandTrace(t *testing.T) {
	source := initializationSource(t)
	gs, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil || assetdata.Hash(gs) != assetdata.IslandIce469085.Source(assetdata.RoleSimulationExecutable).SHA256 {
		t.Fatal("original GS2 unavailable/mismatched", err)
	}
	// Independently fixed original operands, not profile-generated expectations.
	if !bytes.Equal(gs[0xa120+0x31c:0xa120+0x32d], []byte{0x80, 0x3e, 0xde, 0, 0, 0x74, 0x0a, 0x6a, 0x15, 0x9a, 0x0b, 0, 0x44, 8, 0x83, 0xc4, 2}) {
		t.Fatal("source enabled gate / early command15h")
	}
	if binary.LittleEndian.Uint16(source[0x1279:]) != 0xa34 || !bytes.Equal(source[0xc34:0xc3f], []byte{0x8d, 0x0e, 0x6c, 9, 0x8d, 6, 0x78, 9, 0xe9, 0xed, 0xfc}) {
		t.Fatal("source15h constructor mapping")
	}
	if !bytes.Equal(source[0x2690+0x96c:0x2690+0x97c], []byte{0xfc, 0x1b, 0xf9, 0x32, 0xfb, 0xff, 0xfa, 1, 0x13, 0xff, 0xfa, 0, 0x18, 0xff, 0xfd, 0}) {
		t.Fatal("source early prelude/alternate stream")
	}
	// The actual GS2 consumer, not a GS4 opcode-number assumption: its signed
	// F3-based jump table sends these commands to the inspected field writers.
	for op, target := range map[byte]uint16{0xf9: 0x1b7a, 0xfa: 0x1b60, 0xfb: 0x1b46, 0xfc: 0x1b1a, 0xfd: 0x1ac4} {
		if binary.LittleEndian.Uint16(source[0x200+0x1cc0+2*int(op-0xf3):]) != target {
			t.Fatalf("GS2 control%02x consumer differs", op)
		}
	}
	if !bytes.Equal(source[0x200+0x1ac4:0x200+0x1ace], []byte{0x8b, 0x1e, 0xe2, 0x0f, 0x83, 0x7f, 0x1a, 0, 0x75, 0x10}) {
		t.Fatal("FD source alternate-pointer read")
	}
	l := assetdata.IslandIce469085.AudioLayout
	q, err := newAudioSequence(source, l.Drivers["GS2"], l)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.command(0x15); err != nil {
		t.Fatal(err)
	}
	q.interrupt()
	b := q.base(6)
	if q.err != nil || q.mem[b] != 255 || q.mem[b+4] != 19 || q.mem[b+5] != 27 || q.mem[b+1] != 1 || q.mem[b+7] != 255 {
		t.Fatal("source first note/instrument/slide/gate", q.err)
	}
	// Hand: pitch19 %12=7 has FNUM02ff and block1. Key-on writes26h;
	// same-update slide+1 takes combined06ff to0700 while preserving key-on.
	var writes []audioTimedWrite
	for _, w := range q.writes {
		if w.PIT != 0 && (w.Register == 0xa6 || w.Register == 0xb6) {
			writes = append(writes, w)
		}
	}
	want := [][2]byte{{0xa6, 0xff}, {0xb6, 6}, {0xb6, 0x26}, {0xa6, 0}, {0xb6, 0x27}}
	if len(writes) < len(want) {
		t.Fatal("first source register count")
	}
	for i, expected := range want {
		w := writes[len(writes)-len(want)+i]
		if w.Register != expected[0] || w.Value != expected[1] {
			t.Fatalf("source register%d %+v want%v", i, w, expected)
		}
	}
	for q.ticks < 256 {
		q.interrupt()
	}
	if q.mem[b+4] != 24 || q.mem[b+1] != 0 || q.mem[b] != 255 {
		t.Fatal("source stable second note at256", q.mem[b+4], q.mem[b+1], q.mem[b])
	}
	early, err := initializationEarlySequence(source, l.Drivers["GS2"], l)
	if err != nil {
		t.Fatal(err)
	}
	// Initial pair at tick1, next after255, FD after another255. Repeated
	// alternate consists only of the255-duration pair:511/766/1021 updates.
	wantTicks := []uint64{511, 766, 1021}
	for i, tick := range wantTicks {
		if early.loops[6][i] != tick*0x4c90 {
			t.Fatalf("loop%d PIT%d want%d", i, early.loops[6][i], tick*0x4c90)
		}
	}
	if early.busy() == 0 || early.word(l.Drivers["GS2"].NoiseKind) != 0 {
		t.Fatal("source15h falsely terminated or confused with0bh ramp")
	}
}

func TestEarlyEffectLoopReplacementMuteAndGain(t *testing.T) {
	early := &assetdata.AudioClip{Samples: []int16{1000, 2000, 3000}, SampleRate: 49716, LoopStartFrame: 1, LoopEndFrame: 3}
	late := &assetdata.AudioClip{Samples: []int16{4000, 5000}, SampleRate: 49716}
	for _, gain := range []float64{0, .25, 1} {
		p, err := NewAudioPlayer(&assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{InitializationEarlyClip: early, InitializationStartClip: late}}, AudioPreferences{MusicMode: MusicAll, MusicVolume: 1, EffectsVolume: gain})
		if err != nil {
			t.Fatal(err)
		}
		p.Sync("silent", nil, false)
		p.PlayInitializationEarly()
		data := make([]float32, 7)
		p.Render(data, 7)
		for i, sample := range []int16{1000, 2000, 3000, 2000, 3000, 2000, 3000} {
			if data[i] != float32(float64(sample)*gain/32768) {
				t.Fatal("effect sample-clock loop/gain", data)
			}
		}
		p.PlayInitializationStart()
		p.StopInitializationEarly() // Must preserve the replacement rotor tail.
		p.Render(data[:2], 2)
		if data[0] != float32(4000*gain/32768) || p.Status().MusicOwner != "silent" || p.Status().EarlyInitializationCommands != 1 || p.Status().InitializationCommands != 1 {
			t.Fatal("same-owner replacement/cleanup", p.Status(), data)
		}
		p.PlayInitializationEarly()
		p.StopInitializationEarly()
		p.Render(data[:1], 1)
		if data[0] != 0 {
			t.Fatal("failed-loader early loop survived")
		}
		p.SetSoundEffectsEnabled(false)
		p.PlayInitializationEarly()
		p.SetSoundEffectsEnabled(true)
		p.Render(data[:1], 1)
		if data[0] != 0 || p.Status().EarlyInitializationCommands != 2 {
			t.Fatal("muting/re-enable replayed early event")
		}
	}
}
