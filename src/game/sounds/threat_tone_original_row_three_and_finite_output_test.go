package sounds

import (
	"bytes"
	"encoding/hex"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
	"testing"
	"time"
)

func TestThreatToneOriginalRowThreeAndFiniteOutput(t *testing.T) {
	source := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	if !bytes.Equal(source[512+0x717:][:7], []byte{0x8d, 0x1e, 0xe6, 0x0d, 0xeb, 0x13, 0x90}) {
		t.Fatal("original06 row3 veneer")
	}
	raw, _ := hex.DecodeString("8d0eb408e90efe")
	stream, _ := hex.DecodeString("fc0ef93cfb08fa104114fa000000")
	if int(audioWord(source, 512+0x104f+12)) != 0x902 || !bytes.Equal(source[512+0x902:][:7], raw) || !bytes.Equal(source[l.DSFile+0x8b4:][:len(stream)], stream) {
		t.Fatal("original tone06 constructor/stream")
	}
	c, e := prepareGS2Command(source, l, assetdata.IslandIce469085.AudioLayout, 6)
	if e != nil || c.Channel != 3 || c.Start != 0x8b4 || c.Alternate != 0 || c.ClearNoise {
		t.Fatal(c, e)
	}
	q, e := rotorSequence(source[l.DSFile:])
	if e != nil {
		t.Fatal(e)
	}
	if e = q.rotorCommand(15); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 100; n++ {
		q.interrupt()
	}
	rotor := append([]byte(nil), q.mem[q.base(6):q.base(6)+30]...)
	pit, rng := q.pit, q.word(l.RNG)
	if e = q.gs2FixedCommand(c); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(rotor, q.mem[q.base(6):q.base(6)+30]) || q.pit != pit || q.word(l.RNG) != rng {
		t.Fatal("warning reset rotor/clock/RNG")
	}
	finite, _ := rotorSequence(source[l.DSFile:])
	if e = finite.gs2FixedCommand(c); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 10000 && finite.mem[finite.base(3)] != 0; n++ {
		finite.interrupt()
		if finite.err != nil {
			t.Fatal(finite.err)
		}
	}
	keyed := false
	for _, v := range finite.writes {
		if v.Register == 0xb3 && v.Value&0x20 != 0 {
			keyed = true
		}
	}
	if !keyed || finite.mem[finite.base(3)] != 0 {
		t.Fatal("tone06 never keyed or failed to retire")
	}
}

func TestThreatToneLiveWorkerMuteSpeechAndOverlap(t *testing.T) {
	commands, source := combatCommandFixture(t)
	if commands[6].Channel != 3 {
		t.Fatal(commands[6])
	}
	clip := &assetdata.AudioClip{SampleRate: 49716, Samples: []int16{1000, 2000, 3000}}
	a := &assetdata.AudioAssets{GS2Commands: commands, Clips: map[string]*assetdata.AudioClip{"gs2_06": clip}}
	prefs := DefaultAudioPreferences()
	prefs.SpeechMuted = true
	p, e := NewAudioPlayer(a, prefs)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Stop()
	r := &combatFakeRotor{}
	p.rotor = r
	for _, event := range []uint16{6, 0x29, 6} {
		if e = p.PlayGS2Event(event, 0); e != nil {
			t.Fatal(e)
		}
	}
	p.SetSoundEffectsEnabled(false)
	if e = p.PlayGS2Event(6, 0); e != nil {
		t.Fatal(e)
	}
	p.SetEffectsPaused(true)
	if e = p.PlayGS2Event(6, 0); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(r.fixed, []uint16{6, 0x29, 6, 6}) || p.effect != nil {
		t.Fatal(r.fixed, p.effect)
	}
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(true)
	p.rotor = nil
	if e = p.PlayGS2Event(6, 0); e != nil || p.effect != clip {
		t.Fatal(e, p.effect)
	}
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	worker, e := newRotorStream(source[l.DSFile:])
	if e != nil {
		t.Fatal(e)
	}
	defer worker.Close()
	if e = worker.Command(15); e != nil {
		t.Fatal(e)
	}
	if e = worker.GS2Command(commands[6]); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := worker.Status()
		if st.Failure != "" {
			t.Fatal(st.Failure)
		}
		if st.FixedCommands == 1 {
			if st.LastFixedEvent != 6 || st.Commands != 2 {
				t.Fatal(st)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("actual sound sequencer worker did not consume06")
}
