package sounds

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	assetdata "gunship2000-assets/game/assets"
)

func combatCommandFixture(t *testing.T) (map[uint16]assetdata.AudioGS2Command, []byte) {
	t.Helper()
	source := initializationSource(t)
	layout := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	out := map[uint16]assetdata.AudioGS2Command{}
	for _, id := range layout.FixedGameplayEvents {
		c, err := prepareGS2Command(source, layout, assetdata.IslandIce469085.AudioLayout, id)
		if err != nil {
			t.Fatal(err)
		}
		out[uint16(id)] = c
	}
	return out, source
}

func TestGS2CombatActualWorkerOrdersCommandsWithoutRestart(t *testing.T) {
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
	for _, event := range []uint16{7, 9, 8, 0x18, 0xf, 0x10, 0x11, 0x25, 0x27, 0x17, 0x4a} {
		if err = r.GS2Command(commands[event]); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := r.Status()
		if status.Failure != "" {
			t.Fatal(status.Failure)
		}
		if status.FixedCommands == 11 {
			if status.LastFixedEvent != 0x4a || status.Commands != 2 {
				t.Fatal("shared command order or rotor counter", status)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("worker did not consume source fixed-command queue")
}

func TestGS2CombatConstructorsOriginalOperands(t *testing.T) {
	commands, source := combatCommandFixture(t)
	// Exact original table words and complete7-byte wrappers independently
	// establish stream and row. These are GS2, not similarly numbered GS4 cues.
	for _, v := range []struct {
		event               uint16
		handler, start, row int
		raw                 string
	}{
		{3, 0x8ed, 0x8ac, 2, "8d0eac08e91cfe"},
		{7, 0x917, 0x8ec, 1, "8d0eec08e9ebfd"}, {8, 0x925, 0x914, 1, "8d0e1409e9ddfd"},
		{9, 0x92c, 0x91c, 1, "8d0e1c09e9d6fd"}, {0xf, 0xa11, 0x940, 0, "8d0e4009e9eafc"},
		{0x10, 0xa18, 0x948, 0, "8d0e4809e9e3fc"}, {0x14, 0xa46, 0x9a2, 4, "8d0ea209e9d1fc"},
		{0x11, 0xa1f, 0x950, 0, "8d0e5009e9dcfc"},
		{0x18, 0xa3f, 0x988, 1, "8d0e8809e9c3fc"}, {0x25, 0xa7e, 0x9fc, 4, "8d0efc09e999fc"},
		{0x27, 0xa85, 0xa0e, 4, "8d0e0e0ae992fc"}, {0x4a, 0xa93, 0xa1a, 4, "8d0e1a0ae984fc"},
	} {
		c := commands[v.event]
		raw, err := hex.DecodeString(v.raw)
		if err != nil || !bytes.Equal(source[512+v.handler:512+v.handler+7], raw) || c.HandlerCS != v.handler || c.Start != v.start || c.Channel != v.row || c.Alternate != 0 {
			t.Fatalf("event%02x source constructor %+v", v.event, c)
		}
		at := 512 + 0x104f + int(v.event)*2
		if int(audioWord(source, at)) != v.handler {
			t.Fatalf("event%02x original table", v.event)
		}
	}
	// Dispatcher113f XOR AX,AX supplies alternate0 to each simple wrapper.
	if !bytes.Equal(source[512+0x113d:512+0x1146], []byte{0xd1, 0xe3, 0x33, 0xc0, 0x2e, 0xff, 0x97, 0x4f, 0x10}) {
		t.Fatal("original dispatcher changed")
	}
}

func TestGS2CombatSharesRetainedRotorAndSourceRows(t *testing.T) {
	commands, source := combatCommandFixture(t)
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	q, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	if err = q.rotorCommand(1); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 800; i++ {
		q.interrupt()
		if q.err != nil {
			t.Fatal(q.err)
		}
	}
	rotor := append([]byte(nil), q.mem[q.base(6):q.base(6)+30]...)
	rng, pit, speed, step := q.word(l.RNG), q.pit, q.word(l.NoiseSpeed), q.word(l.NoiseStep)
	if err = q.gs2FixedCommand(commands[7]); err != nil {
		t.Fatal(err)
	}
	if err = q.gs2FixedCommand(commands[9]); err != nil {
		t.Fatal(err)
	}
	if q.word(q.base(1)+16) != 0x91c || q.mem[q.base(1)] != 1 {
		t.Fatal("rocket failed source row1 replacement")
	}
	if err = q.gs2FixedCommand(commands[0x18]); err != nil {
		t.Fatal(err)
	}
	if err = q.gs2FixedCommand(commands[0xf]); err != nil {
		t.Fatal(err)
	}
	if q.word(q.base(0)+16) != 0x940 {
		t.Fatal("destruction failed source row0 replacement")
	}
	if !bytes.Equal(rotor, q.mem[q.base(6):q.base(6)+30]) || q.word(l.RNG) != rng || q.pit != pit || q.word(l.NoiseSpeed) != speed || q.word(l.NoiseStep) != step {
		t.Fatal("fixed combat command reset independent rotor/clock/RNG history")
	}
	for i := 0; i < 100; i++ {
		q.interrupt()
	}
	if q.err != nil || q.word(l.NoiseKind) != 1 || q.pit <= pit {
		t.Fatal("shared sequence stopped rotor", q.err)
	}
}

func TestGS2ProjectileViewCommandOriginalNoiseAndAlternate(t *testing.T) {
	commands, source := combatCommandFixture(t)
	c := commands[0x17]
	if c.HandlerCS != 0x9f5 || c.HandlerHex != "a3920c8d0e7c098d068409e929fd" || c.Channel != 6 || c.Start != 0x97c || c.Alternate != 0x984 || !c.ClearNoise {
		t.Fatal("original projectile-view wrapper", c)
	}
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	if audioWord(source, 512+l.EventTableCS+0x17*2) != 0x9f5 {
		t.Fatal("source17 table word")
	}
	q, err := rotorSequence(source[l.DSFile:])
	if err != nil {
		t.Fatal(err)
	}
	if err = q.rotorCommand(1); err != nil {
		t.Fatal(err)
	}
	q.putword(l.NoiseKind, 1)
	rng, pit := q.word(l.RNG), q.pit
	if err = q.gs2FixedCommand(c); err != nil {
		t.Fatal(err)
	}
	if q.word(l.NoiseKind) != 0 || q.word(q.base(6)+14) != 0x97c || q.word(q.base(6)+26) != 0x984 || q.word(l.RNG) != rng || q.pit != pit {
		t.Fatal("source17 exact state ownership")
	}
	// Source expiry returns to0bh, resuming the retained driver's own rotor
	// controller rather than restarting a detached WAV or every FM channel.
	if err = q.rotorCommand(10); err != nil {
		t.Fatal(err)
	}
	if q.word(l.NoiseKind) != 1 {
		t.Fatal("source rotor restoration")
	}
	clip := &assetdata.AudioClip{Samples: make([]int16, 12), SampleRate: 49716, LoopStartFrame: 4, LoopEndFrame: 12}
	p, err := NewAudioPlayer(&assetdata.AudioAssets{GS2Commands: map[uint16]assetdata.AudioGS2Command{0x17: c}, Clips: map[string]*assetdata.AudioClip{c.TrackKey: clip}}, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if err = p.PlayGS2Event(0x17, 0); err != nil || !p.effectLoop {
		t.Fatal("isolated sustained fallback", err)
	}
	p.UpdateRotor(10)
	if p.effect != nil || p.effectLoop || p.combatFallback {
		t.Fatal("0bh failed to replace fallback17 without live helper")
	}
}

type combatFakeRotor struct {
	rotorPCM
	fixed []uint16
}

func (r *combatFakeRotor) GS2Command(c assetdata.AudioGS2Command) error {
	r.fixed = append(r.fixed, c.EventID)
	return nil
}

func TestGS2GameplayPlayerDispatchMutePauseAndFallback(t *testing.T) {
	commands, _ := combatCommandFixture(t)
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{}, GS2Commands: commands}
	for _, c := range commands {
		a.Clips[c.TrackKey] = &assetdata.AudioClip{Samples: []int16{1000, 2000, 3000}, SampleRate: 49716}
	}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	r := &combatFakeRotor{}
	p.rotor = r
	for _, event := range []uint16{7, 9, 8, 0x18, 0xf, 0x10} {
		if err = p.PlayGS2Event(event, 0); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(r.fixed, []uint16{7, 9, 8, 0x18, 0xf, 0x10}) || p.effect != nil {
		t.Fatal("live dispatch doubled or reordered through PCM")
	}
	p.SetSoundEffectsEnabled(false)
	if err = p.PlayGS2Event(7, 0); err != nil || len(r.fixed) != 7 {
		t.Fatal("muted shared state command lost", err)
	}
	p.SetEffectsPaused(true)
	if err = p.PlayGS2Event(9, 0); err != nil || len(r.fixed) != 7 {
		t.Fatal("suspended gameplay sound progressed", err)
	}
	p.SetEffectsPaused(false)
	if err = p.PlayGS2Event(0x42, 0); err == nil {
		t.Fatal("GS4 number silently accepted as GS2 gameplay")
	}
	if err = p.PlayGS2Event(7, 1); err == nil {
		t.Fatal("unrecovered fixed parameter accepted")
	}
	p.SetSoundEffectsEnabled(true)
	p.rotor = nil
	if err = p.PlayGS2Event(9, 0); err != nil {
		t.Fatal(err)
	}
	if p.effect != a.Clips["gs2_09"] || p.effectFrames != 0 {
		t.Fatal("bounded fallback missing")
	}
	p.prefs.EffectsVolume = 0
	dst := make([]float32, 3)
	p.Render(dst, 3)
	if !reflect.DeepEqual(dst, []float32{0, 0, 0}) || p.effectFrames != 3 {
		t.Fatal("volume0 failed gain/history")
	}
	p.Stop()
	if err = p.PlayGS2Event(7, 0); err != nil {
		t.Fatal(err)
	}
	if p.Status().CombatCommands != 8 {
		t.Fatal("paused/invalid/closed request was counted")
	}
}
