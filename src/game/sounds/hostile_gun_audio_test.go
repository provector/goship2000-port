package sounds

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	assetdata "gunship2000-assets/game/assets"
)

func TestGunAudioOriginalConstructorAndSharedRotor(t *testing.T) {
	source := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	raw, _ := hex.DecodeString("8d0ee008e9f2fd")
	if int(audioWord(source, 512+0x104f+2*0x29)) != 0x910 || !bytes.Equal(source[512+0x910:][:7], raw) || !bytes.Equal(source[512+0x709:][:6], []byte{0x8d, 0x1e, 0xaa, 0x0d, 0xeb, 0x21}) {
		t.Fatal("source29 table/wrapper/row1 veneer changed")
	}
	stream, _ := hex.DecodeString("fc2ff932ff002903ff070000")
	if !bytes.Equal(source[l.DSFile+0x8e0:][:len(stream)], stream) {
		t.Fatal("source hostile gun stream changed")
	}
	c, err := prepareGS2Command(source, l, assetdata.IslandIce469085.AudioLayout, 0x29)
	if err != nil || c.Channel != 1 || c.Start != 0x8e0 || c.Alternate != 0 || c.ClearNoise || c.HandlerHex != "8d0ee008e9f2fd" || c.TrackKey != "gs2_29" {
		t.Fatal(c, err)
	}
	q, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	if err = q.rotorCommand(15); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 800; i++ {
		q.interrupt()
		if q.err != nil {
			t.Fatal(q.err)
		}
	}
	rotor := append([]byte(nil), q.mem[q.base(6):q.base(6)+30]...)
	pit, rng := q.pit, q.word(l.RNG)
	if err = q.gs2FixedCommand(c); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rotor, q.mem[q.base(6):q.base(6)+30]) || q.word(q.base(1)+14) != 0x8e0 || q.pit != pit || q.word(l.RNG) != rng {
		t.Fatal("gun29 reset rotor/clock/RNG or wrong channel")
	}
	// Later source row1 commands replace only row1. This preserves genuine
	// source ordering when gun29 overlaps rocket/weapon cues on that row.
	commands, _ := combatCommandFixture(t)
	if err = q.gs2FixedCommand(commands[7]); err != nil {
		t.Fatal(err)
	}
	if q.word(q.base(1)+14) != 0x8ec || !bytes.Equal(rotor, q.mem[q.base(6):q.base(6)+30]) {
		t.Fatal("source later row1 replacement")
	}
	finite, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	if err = finite.gs2FixedCommand(c); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000 && finite.mem[finite.base(1)] != 0; i++ {
		finite.interrupt()
		if finite.err != nil {
			t.Fatal(finite.err)
		}
	}
	keyed := false
	for _, w := range finite.writes {
		if w.Register == 0xb1 && w.Value&0x20 != 0 {
			keyed = true
		}
	}
	if !keyed || finite.mem[finite.base(1)] != 0 {
		t.Fatal("gun29 did not produce a finite keyed row1 stream", keyed)
	}
}

func TestGunAudioLiveDispatchMutePauseAndExactFallback(t *testing.T) {
	commands, _ := combatCommandFixture(t)
	clip := &assetdata.AudioClip{SampleRate: 49716, Samples: []int16{1000, 2000, 3000}}
	a := &assetdata.AudioAssets{GS2Commands: commands, Clips: map[string]*assetdata.AudioClip{"gs2_29": clip}}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	r := &combatFakeRotor{}
	p.rotor = r
	if err = p.PlayGS2Event(0x29, 0); err != nil {
		t.Fatal(err)
	}
	p.SetSoundEffectsEnabled(false)
	if err = p.PlayGS2Event(0x29, 0); err != nil {
		t.Fatal(err)
	}
	p.SetEffectsPaused(true)
	if err = p.PlayGS2Event(0x29, 0); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.fixed, []uint16{0x29, 0x29}) || p.effect != nil {
		t.Fatal("shared command lost/duplicated or pause advanced", r.fixed, p.effect)
	}
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(true)
	p.rotor = nil
	if err = p.PlayGS2Event(0x29, 0); err != nil || p.effect != clip || p.effectFrames != 0 {
		t.Fatal("exact source29 fallback missing", err, p.effect)
	}
	if err = p.PlayGS2Event(0x29, 1); err == nil {
		t.Fatal("parameterized command invented")
	}
}

func TestActualSoundWorkerConsumesGun29WithoutRestart(t *testing.T) {
	commands, source := combatCommandFixture(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	r, err := newRotorStream(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = r.Command(15); err != nil {
		t.Fatal(err)
	}
	if err = r.GS2Command(commands[0x29]); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := r.Status()
		if status.Failure != "" {
			t.Fatal(status.Failure)
		}
		if status.FixedCommands == 1 {
			if status.LastFixedEvent != 0x29 || status.Commands != 2 {
				t.Fatal("gun command reset/replaced rotor owner", status)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("sound worker did not consume source29")
}
