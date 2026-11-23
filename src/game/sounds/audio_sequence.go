package sounds

import (
	"encoding/binary"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/audioopl"
	"math/bits"
	"sort"
)

const audioFMRate = 49716

type audioTimedWrite struct {
	PIT      uint64 `json:"pit_clocks"`
	Register byte   `json:"register"`
	Value    byte   `json:"value"`
}

type audioSequence struct {
	layout           assetdata.AudioDriverLayout
	clocks           *assetdata.AudioSourceLayout
	source           []byte
	mem              [65536]byte
	known            [65536]bool
	ticks, irqs, pit uint64
	fast             bool
	divider          byte
	writes           []audioTimedWrite
	streams          map[int]bool
	loops            map[int][]uint64
	err              error
	localWorker      byte // GS2 uses BP-4, not a source DS flag.
}

func audioWord(b []byte, at int) uint16 { return binary.LittleEndian.Uint16(b[at : at+2]) }

func (q *audioSequence) word(at int) uint16 {
	return uint16(q.mem[at&65535]) | uint16(q.mem[(at+1)&65535])<<8
}

func (q *audioSequence) putword(at int, v uint16) {
	q.mem[at&65535] = byte(v)
	q.mem[(at+1)&65535] = byte(v >> 8)
}

func (q *audioSequence) base(ch int) int { return q.layout.Channels + q.layout.Stride*ch }

func (q *audioSequence) setWorkerFlag(v byte) {
	if q.layout.WorkerFlag < 0 {
		q.localWorker = v
		return
	}
	q.mem[q.layout.WorkerFlag] = v
}
func (q *audioSequence) workerFlag() byte {
	if q.layout.WorkerFlag < 0 {
		return q.localWorker
	}
	return q.mem[q.layout.WorkerFlag]
}

func (q *audioSequence) rng() uint16 {
	n := bits.RotateLeft16(q.word(q.layout.RNG)+0x9248, -3)
	q.putword(q.layout.RNG, n)
	return n
}

func (q *audioSequence) sampleFrame() uint64 {
	return q.pit * audioFMRate / uint64(q.clocks.PITFrequency)
}

func (q *audioSequence) write(reg, val int) {
	r, v := byte(reg), byte(val)
	q.mem[q.layout.Shadow+int(r)] = v
	q.writes = append(q.writes, audioTimedWrite{q.pit, r, v})
}

func (q *audioSequence) stream(at uint16) byte {
	if !q.known[int(at)] {
		q.err = fmt.Errorf("%s sequence reaches unsupported/uninitialized DS:%04x", q.layout.Name, at)
		return 0
	}
	q.streams[int(at)] = true
	return q.mem[at]
}

func (q *audioSequence) busy() byte {
	var n byte
	for ch := 0; ch < 8; ch++ {
		n |= q.mem[q.base(ch)]
	}
	return n
}

func newAudioSequence(source []byte, l assetdata.AudioDriverLayout, c *assetdata.AudioSourceLayout) (*audioSequence, error) {
	if len(source) < 512 || string(source[:2]) != "MZ" || int(audioWord(source, 8))*16 != 512 || l.DSFile < 512 || l.DSFile >= len(source) || len(source)-l.DSFile > 65536 {
		return nil, fmt.Errorf("%s: invalid recognized MZ/data framing", l.Name)
	}
	q := &audioSequence{source: source, layout: l, clocks: c, divider: 3, streams: map[int]bool{}, loops: map[int][]uint64{}}
	copy(q.mem[:], source[l.DSFile:])
	for i := 0; i < len(source)-l.DSFile; i++ {
		q.known[i] = true
	}
	// Source initialized bytes, including the private53h stack pattern, survive.
	// Only otherwise uninitialized storage uses this named deterministic zero policy.
	if l.Backend >= 0 {
		q.putword(l.Backend, 2)
		if l.Name == "GS3" {
			q.putword(l.Backend, 3)
		}
		q.mem[l.Probe] = 255
	}
	if q.word(l.RNG) != 1234 {
		return nil, fmt.Errorf("%s: source RNG initializer is not04d2h", l.Name)
	}
	q.reset()
	return q, nil
}

func (q *audioSequence) reset() {
	if q.layout.Name == "LOG" {
		for i := 0; i < 8; i++ {
			b := q.base(i)
			q.mem[b] = 0
			q.mem[b+1] = 0
			q.mem[b+2] = 0
		}
		for i := 0; i < 6; i++ {
			q.mem[q.layout.Life+i] = 0
		}
		for i := 0; i < 8; i++ {
			q.mem[q.layout.Bases+i] = 0
		}
		q.mem[q.layout.UpdateEnabled] = 255
	} else {
		q.mem[q.layout.UpdateEnabled] = 0
		for i := 0; i < 8; i++ {
			b := q.base(i)
			for j := 0; j < 4; j++ {
				q.mem[b+j] = 0
			}
		}
		q.mem[q.layout.Life] = 0
		q.mem[q.layout.Life+1] = 0
		for _, at := range []int{q.layout.Masks, q.layout.Masks + 2, q.layout.Bases, q.layout.Bases + 2, q.layout.Deltas, q.layout.Deltas + 2, q.layout.NoiseKind, q.layout.NoiseSpeed, q.layout.NoiseStep, q.layout.NoiseArgument} {
			q.putword(at, 0)
		}
		q.mem[q.layout.NoiseLevel] = 50
		q.mem[q.layout.UpdateEnabled] = 255
	}
	for r := 0x4f; r > 0x3f; r-- {
		q.write(r, 63)
	}
	for r := 255; r > 0x5f; r-- {
		q.write(r, 0)
	}
	for r := 0x3f; r > 0; r-- {
		q.write(r, 0)
	}
	q.write(1, 0x20)
}

func (q *audioSequence) initialize(ch, start, alternate int) {
	if ch < 0 || ch >= 7 || start < 0 || start >= 65536 {
		q.err = fmt.Errorf("invalid %s channel constructor", q.layout.Name)
		return
	}
	b := q.base(ch)
	for _, pair := range [][2]int{{1, 0}, {10, 255}, {2, 0}, {3, 0}, {28, 0}, {29, 0}, {13, 64}, {7, 0}} {
		q.mem[b+pair[0]] = byte(pair[1])
	}
	for _, at := range []int{14, 16, 18, 20} {
		q.putword(b+at, uint16(start))
	}
	q.putword(b+22, 0)
	q.putword(b+24, 0)
	q.putword(b+26, uint16(alternate))
	q.mem[b] = 1
}

func (q *audioSequence) command(event int) error {
	if event < 0 || event >= q.layout.EventCount {
		return fmt.Errorf("%s event outside source table", q.layout.Name)
	}
	if q.layout.Name == "LOG" {
		if event != 2 {
			return fmt.Errorf("unsupported LOG preparation command%d", event)
		}
		for ch, start := range q.layout.LogoStarts {
			b := q.base(ch)
			for _, f := range []int{1, 2, 6, 24, 25} {
				q.mem[b+f] = 0
			}
			q.mem[b+9] = 255
			for _, f := range []int{10, 12, 14, 16} {
				q.putword(b+f, uint16(start))
			}
			for _, f := range []int{18, 20, 22} {
				q.putword(b+f, 0)
			}
			q.mem[b] = 1
		}
		return nil
	}
	table := 512 + q.layout.EventTableCS + event*2
	if table+2 > len(q.source) {
		return fmt.Errorf("short event table")
	}
	at := int(audioWord(q.source, table))
	var cx, ax, bx int
	for count := 0; count < 64; count++ {
		f := 512 + at
		if f < 0 || f+8 > len(q.source) {
			return fmt.Errorf("constructor bounds")
		}
		b := q.source[f : f+8]
		switch {
		case b[0] == 0x8d && b[1] == 0x0e:
			cx = int(audioWord(b, 2))
			at += 4
		case b[0] == 0x8d && b[1] == 0x06:
			ax = int(audioWord(b, 2))
			at += 4
		case b[0] == 0xa3:
			q.putword(int(audioWord(b, 1)), uint16(ax))
			at += 3
		case b[0] == 0x8d && b[1] == 0x1e:
			bx = int(audioWord(b, 2))
			at += 4
		case b[0] == 0xe8 || b[0] == 0xe9:
			target := int(uint16(at + 3 + int(int16(audioWord(b, 1)))))
			index := -1
			if target == q.layout.InitCommonCS {
				index = (bx - q.layout.Channels) / 30
			} else if target >= q.layout.FirstInitCS && target <= q.layout.FirstInitCS+42 && (target-q.layout.FirstInitCS)%7 == 0 {
				index = (target - q.layout.FirstInitCS) / 7
			}
			if index < 0 || index > 6 {
				return fmt.Errorf("%s unsupported constructor target%04x", q.layout.Name, target)
			}
			q.initialize(index, cx, ax)
			at += 3
			if b[0] == 0xe9 {
				return q.err
			}
		case b[0] == 0xc3:
			return q.err
		default:
			return fmt.Errorf("%s unsupported constructor opcode%02x atCS%04x", q.layout.Name, b[0], at)
		}
	}
	return fmt.Errorf("constructor diagnostic bound reached")
}

func (q *audioSequence) update(ch int) {
	b := q.base(ch)
	q.putword(q.layout.Pointer, uint16(b))
	q.mem[q.layout.Ordinal] = byte(ch)
	if q.mem[b] == 0 {
		return
	}
	if q.mem[b+8] != 0 {
		q.mem[b+8]--
		if q.mem[b+8] == 0 {
			q.keyoff(ch)
		}
	}
	q.mem[b]--
	if q.mem[b] == 0 {
		done := false
		for operations := 0; operations < 4096; operations++ {
			pos := q.word(b + 16)
			op := q.stream(pos)
			if q.err != nil {
				return
			}
			if op <= 0xf2 {
				done = true
				break
			}
			q.setWorkerFlag(0)
			dirty := false
			operand := func(k uint16) byte { return q.stream(pos + k) }
			next := func(n uint16) { q.putword(b+16, pos+n) }
			switch op {
			case 0xff, 0xfe:
				counter, anchor := 22, 18
				if op == 0xfe {
					counter, anchor = 24, 20
				}
				v := q.word(b + counter)
				if v == 0 {
					n := operand(1)
					if n == 0 {
						next(2)
						q.putword(b+anchor, pos+2)
						q.putword(b+counter, 0)
						if op == 0xfe {
							q.putword(b+18, pos+2)
							q.putword(b+22, 0)
						}
					} else {
						q.putword(b+counter, uint16(int16(int8(n))))
						q.putword(b+16, q.word(b+anchor))
					}
				}
				if v != 0 {
					v--
					q.putword(b+counter, v)
					if v == 0 {
						next(2)
						q.putword(b+anchor, pos+2)
						if op == 0xfe {
							q.putword(b+18, pos+2)
						}
					} else {
						q.putword(b+16, q.word(b+anchor))
					}
				}
			case 0xfd:
				target := q.word(b + 26)
				if target != 0 {
					for _, f := range []int{14, 16, 18, 20} {
						q.putword(b+f, target)
					}
				} else {
					q.putword(b+16, q.word(b+14))
				}
				q.loops[ch] = append(q.loops[ch], q.pit)
			case 0xf6:
				n := int(int8(operand(1)))
				q.rng()
				selection := int(uint16(n-1) & q.word(q.layout.RNG))
				chosen := q.stream(uint16(int(pos) + 2 + selection))
				displacement := int(int8(q.stream(uint16(int(pos) + 2 + n))))
				dest := uint16(int(pos) + n + 3 + displacement)
				q.mem[dest] = chosen
				q.known[dest] = true
				q.putword(b+16, uint16(int(pos)+n+3))
			case 0xfc:
				q.mem[b+5] = operand(1)
				next(2)
				q.instrument(ch, int(q.mem[b+5]))
			case 0xfb:
				q.mem[b+7] = operand(1)
				next(2)
			case 0xfa:
				q.mem[b+1] = operand(1)
				next(2)
			case 0xf9:
				q.mem[b+6] = operand(1)
				next(2)
				dirty = true
			case 0xf8:
				q.mem[b+10] = operand(1)
				q.mem[b+2] = operand(2)
				q.mem[b+9] = 1
				next(3)
			case 0xf7:
				q.mem[b+28] = operand(1)
				next(2)
			case 0xf5:
				q.mem[b+29] = operand(1)
				next(2)
			case 0xf4:
				q.mem[b+13] = operand(1)
				next(2)
				dirty = true
			case 0xf3:
				q.mem[b+12] = operand(1)
				q.mem[b+3] = operand(2)
				q.mem[b+11] = 1
				next(3)
			default:
				q.err = fmt.Errorf("unsupported control%02x", op)
				return
			}
			if dirty {
				q.setWorkerFlag(1)
			}
			if q.err != nil {
				return
			}
		}
		if !done {
			q.err = fmt.Errorf("%s control execution bound", q.layout.Name)
			return
		}
		if q.workerFlag() != 0 {
			q.level(ch)
		}
		pos := q.word(b + 16)
		note, duration := q.stream(pos), q.stream(pos+1)
		q.mem[b+4] = note
		q.mem[b] = duration
		q.putword(b+16, pos+2)
		if note == 0 || duration == 0 {
			q.keyoff(ch)
		} else {
			q.mem[b+8] = byte(int(duration) - int(int8(q.mem[b+7])))
			q.note(ch)
		}
	}
	if q.mem[b+1] != 0 {
		q.frequencySlide(ch)
	}
	dirty := false
	q.setWorkerFlag(0)
	// Both counters decrement when either is nonzero, including the companion's
	// byte wrap. Independent counters would alter original slide/gate ordering.
	if q.mem[b+9] != 0 || q.mem[b+11] != 0 {
		q.mem[b+9]--
		if q.mem[b+9] == 0 {
			q.mem[b+9] = q.mem[b+10]
			if q.mem[b+2] != 0 {
				q.mem[b+29] += q.mem[b+2]
				dirty = true
			}
		}
		q.mem[b+11]--
		if q.mem[b+11] == 0 {
			q.mem[b+11] = q.mem[b+12]
			if q.mem[b+3] != 0 {
				q.mem[b+13] += q.mem[b+3]
				dirty = true
			}
		}
	}
	if dirty {
		q.setWorkerFlag(1)
		q.level(ch)
	}
}

func (q *audioSequence) tick() {
	q.ticks++
	q.rng()
	if q.mem[q.layout.UpdateEnabled] == 0 {
		return
	}
	// GS2's initialization0bh uses the same channel machinery, with its own
	// source-backed refresh producer, shared by the bounded bake and live worker.
	// Semantic parameter commands arrive separately; drawing never calls them.
	if q.word(q.layout.NoiseKind) != 0 {
		if q.layout.Name != "GS2" {
			q.err = fmt.Errorf("parameterized simulation channel6 is outside frontend baking")
			return
		}
		q.refreshInitializationChannel()
	}
	for ch := 0; ch < 7; ch++ {
		q.update(ch)
	}
	for slot := 0; slot < 2; slot++ {
		life := q.layout.Life + slot
		if q.mem[life] != 0 {
			q.putword(q.layout.Bases+slot*2, q.word(q.layout.Bases+slot*2)+q.word(q.layout.Deltas+slot*2))
			q.mem[life]--
			if q.mem[life] == 0 {
				other := 1 - slot
				hardware := q.mem[q.layout.Owners+slot]
				off := false
				if slot == 0 {
					off = q.mem[q.layout.Life+other] == 0 || hardware != q.mem[q.layout.Owners+other]
				} else {
					off = q.mem[q.layout.Life] != 0 || hardware != q.mem[q.layout.Owners]
				}
				if off {
					q.offHardware(int(hardware))
				}
			}
		}
	}
	if q.mem[q.layout.Life]|q.mem[q.layout.Life+1] == 0 {
		q.setFast(false)
	}
}

func (q *audioSequence) setFast(fast bool) {
	if q.layout.Name == "GS3" && fast {
		q.err = fmt.Errorf("unexpected noise clock request in finite title profile")
		return
	}
	if q.fast && !fast {
		q.divider = 3
	}
	q.fast = fast
}

func (q *audioSequence) interrupt() {
	q.irqs++
	if q.layout.Name == "LOG" {
		q.logoInterrupt()
		return
	}
	if q.layout.Name == "GS3" {
		q.pit += uint64(q.clocks.PlayerFallbackReload)
		q.tick()
		return
	}
	wasFast := q.fast
	if wasFast {
		q.pit += uint64(q.clocks.FastReload)
		q.noiseTick()
		q.divider--
		if q.divider == 0 {
			q.divider = 5
			q.tick()
		}
	} else {
		q.pit += uint64(q.clocks.BaselineReload)
		q.tick()
		q.divider--
		if q.divider == 0 {
			q.divider = 3
		}
	}
}

func (q *audioSequence) sourceRanges() []assetdata.AudioSourceRange {
	keys := make([]int, 0, len(q.streams))
	for at := range q.streams {
		keys = append(keys, at)
	}
	sort.Ints(keys)
	var out []assetdata.AudioSourceRange
	for _, at := range keys {
		if len(out) > 0 && out[len(out)-1].End == at {
			out[len(out)-1].End++
		} else {
			out = append(out, assetdata.AudioSourceRange{at, at + 1})
		}
	}
	return out
}

func (q *audioSequence) componentWrites() []audioopl.Write {
	out := make([]audioopl.Write, len(q.writes))
	for i, w := range q.writes {
		out[i] = audioopl.Write{Frame: w.PIT * audioFMRate / uint64(q.clocks.PITFrequency), Register: uint16(w.Register), Value: w.Value}
	}
	return out
}
