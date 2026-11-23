package sounds

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
)

const InitializationStartClip = "gs2_initialization_start"
const InitializationEarlyClip = "gs2_initialization_early"

// Source0323 calls15h before loader messages. Its row6 prelude slides note19,
// then FD restarts the stable note24 at the constructor's alternate pointer.
// Three observed source loop boundaries bound extraction; this does not turn
// the state-dependent source cue into a finite song.
func initializationEarlySequence(source []byte, l assetdata.AudioDriverLayout, c *assetdata.AudioSourceLayout) (*audioSequence, error) {
	if l.Name != "GS2" {
		return nil, fmt.Errorf("early initialization requires GS2's source profile")
	}
	q, err := newAudioSequence(source, l, c)
	if err != nil {
		return nil, err
	}
	if err = q.command(0x15); err != nil {
		return nil, err
	}
	for i := 0; i < 30000; i++ {
		q.interrupt()
		if q.err != nil {
			return nil, q.err
		}
		if len(q.loops[6]) >= 3 {
			return q, nil
		}
	}
	return nil, fmt.Errorf("early GS2 initialization did not reach three source loop boundaries")
}

// ASOUND.GS2:0941 is the parameterized0bh constructor with argument1 used by
// GS2's initial scene setup. Its row6 stream repeats through a mutable duration;
// it is not a finite GS4 menu event or the no-op GS2 command2bh.
func (q *audioSequence) startInitializationChannel() error {
	l := q.layout
	if l.Name != "GS2" || q.word(l.NoiseKind) != 0 || l.InitializationAlternate != l.InitializationStream+4 {
		return fmt.Errorf("initialization sound requires a reset GS2 source profile")
	}
	q.putword(l.NoiseKind, 1)
	q.mem[l.InitializationAlternate+1] = q.mem[l.InitializationDurations]
	q.known[l.InitializationAlternate+1] = true
	q.mem[l.InitializationStream+3] = q.mem[l.NoiseLevel]
	q.known[l.InitializationStream+3] = true
	q.initialize(6, l.InitializationStream, l.InitializationAlternate)
	// Source094e uses the old speed/level before storing argument1 and50+(1>>1).
	q.mem[l.NoiseLevel] = 50
	q.putword(l.NoiseStep, 1)
	q.putword(l.NoiseArgument, 1)
	return q.err
}

// Source04b1..0514 runs before the seven channel updates. A completed row6
// restarts at its alternate note pair, without reinstalling the instrument.
func (q *audioSequence) refreshInitializationChannel() {
	l := q.layout
	if q.word(l.NoiseKind) != 1 {
		q.err = fmt.Errorf("unsupported GS2 initialization sound kind")
		return
	}
	b := q.base(6)
	if q.mem[b] != 0 {
		return
	}
	speed := int16(q.word(l.NoiseSpeed))
	step := int16(q.word(l.NoiseStep))
	if step != 0 {
		speed += step
		if speed < 0 {
			speed = 0
			q.putword(l.NoiseStep, 0)
			q.putword(l.NoiseKind, 0)
			return
		}
		if uint16(speed) > 21 {
			speed = 21
			q.putword(l.NoiseStep, 0)
		}
	}
	q.putword(l.NoiseSpeed, uint16(speed))
	duration := q.mem[l.InitializationDurations+int(speed)]
	// Both source presets share the same mutable timing producer (0909/0911).
	for _, at := range []int{l.InitializationAlternate + 1, l.InitializationAlternate + 9} {
		q.mem[at] = duration
		q.known[at] = true
	}
	q.mem[b+6] = q.mem[l.NoiseLevel]
	restart := q.word(b + 26)
	q.putword(b+16, restart)
	q.putword(b+14, restart)
	q.mem[b] = 1
}

// The source0bh sound sustains after the ramp. This preview-stage bake stops at
// the first clamped steady index21 using source command0's reset/release, rather
// than adding continuous inflight rotor audio. Timing uses the source PIT clock.
func initializationSoundSequence(source []byte, l assetdata.AudioDriverLayout, c *assetdata.AudioSourceLayout) (*audioSequence, error) {
	q, err := newAudioSequence(source, l, c)
	if err != nil {
		return nil, err
	}
	if err = q.startInitializationChannel(); err != nil {
		return nil, err
	}
	for i := 0; i < 30000; i++ {
		q.interrupt()
		if q.err != nil {
			return nil, q.err
		}
		if q.word(l.NoiseSpeed) == 21 && q.word(l.NoiseStep) == 0 {
			q.reset()
			return q, nil
		}
	}
	return nil, fmt.Errorf("GS2 initialization ramp exceeded diagnostic IRQ bound")
}
