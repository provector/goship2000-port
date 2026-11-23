package sounds

import (
	"fmt"
)

func (q *audioSequence) logoChannel(ch int) {
	b := q.channel(ch)
	if q.mem[b] != 0 {
		if q.mem[b+7] != 0 {
			q.mem[b+7]--
			if q.mem[b+7] == 0 {
				q.keyoff(ch)
			}
		}
		q.mem[b]--
		if q.mem[b] == 0 {
			complete := false
			for count := 0; count < 1024; count++ {
				at := q.word(b + 12)
				op := q.stream(at)
				if q.err != nil {
					return
				}
				if op < 0xf5 {
					note, duration := op, q.stream(at+1)
					q.mem[b+3] = note
					q.mem[b] = duration
					q.putword(b+12, at+2)
					if note == 0 || duration == 0 {
						q.keyoff(ch)
					} else {
						q.mem[b+7] = byte(int(duration) - int(int8(q.mem[b+6])))
						q.note(ch)
					}
					complete = true
					break
				}
				if op == 0xf6 || op >= 0xfd {
					q.err = fmt.Errorf("LOG unsupported stateful control%02x (not reached by this logo)", op)
					return
				}
				v := q.stream(at + 1)
				length := uint16(2)
				switch op {
				case 0xf5:
					q.mem[b+25] = v
				case 0xf7:
					q.mem[b+24] = v
				case 0xf8:
					q.mem[b+9] = v
					q.mem[b+2] = q.stream(at + 2)
					q.mem[b+8] = 1
					length = 3
				case 0xf9:
					q.mem[b+5] = v
				case 0xfa:
					q.mem[b+1] = v
				case 0xfb:
					q.mem[b+6] = v
				case 0xfc:
					q.mem[b+4] = v
				default:
					q.err = fmt.Errorf("unsupported LOG control%02x", op)
					return
				}
				q.putword(b+12, at+length)
				if op == 0xfc {
					q.instrument(ch, int(v))
				}
				if op == 0xf9 {
					q.level(ch)
				}
			}
			if !complete {
				q.err = fmt.Errorf("LOG control bound reached")
				return
			}
		}
	}
	// LOG reaches these slides even on inactive channels, unlike GS3/GS4.
	if q.mem[b+1] != 0 {
		q.frequencySlide(ch)
	}
	if q.mem[b+8] != 0 {
		q.mem[b+8]--
		if q.mem[b+8] == 0 {
			q.mem[b+8] = q.mem[b+9]
			if q.mem[b+2] != 0 {
				q.mem[b+25] += q.mem[b+2]
				q.level(ch)
			}
		}
	}
}

func (q *audioSequence) logoInterrupt() {
	if q.fast {
		q.err = fmt.Errorf("unexpected LOG noise path in the six-stream logo")
		return
	}
	q.pit += uint64(q.clocks.BaselineReload)
	q.ticks++
	q.rng()
	for ch := 0; ch < 7; ch++ {
		q.logoChannel(ch)
	}
	for slot := 0; slot < 2; slot++ {
		if q.mem[q.layout.Life+slot] != 0 {
			q.err = fmt.Errorf("unexpected active LOG noise in finite logo")
			return
		}
	}
	q.divider--
	if q.divider == 0 {
		q.divider = 3
	}
}
