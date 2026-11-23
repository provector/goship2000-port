package sounds

import (
	"fmt"
)

// These helpers reproduce the selected mono388/389 register layer. They feed
// the pinned DBOPL component in offline bakes and the isolated GS2 flight worker.
// Neither path performs DOS I/O or executes the original driver.
func (q *audioSequence) channel(ch int) int {
	if ch < 0 || ch > 8 {
		q.err = fmt.Errorf("audio hardware channel out of range")
		return q.layout.Channels
	}
	q.mem[q.layout.Ordinal] = byte(ch)
	b := q.base(ch)
	q.putword(q.layout.Pointer, uint16(b))
	return b
}

func (q *audioSequence) operator(ch, half int) int {
	logical := int(int8(q.mem[q.layout.Pairs+ch*2+half]))
	return int(int8(q.mem[uint16(q.layout.Operators+logical)]))
}

func (q *audioSequence) keyoff(ch int) {
	q.channel(ch)
	q.write(0xb0+ch, int(q.mem[q.layout.Shadow+0xb0+ch]&0xdf))
}

func (q *audioSequence) offHardware(ch int) { q.write(0xa0+ch, 0); q.write(0xb0+ch, 0) }

func (q *audioSequence) instrumentHalf(ch, instrument, half int) {
	at := 0x50 + instrument*44 + half*22
	q.putword(q.layout.InstrumentPointer, uint16(at))
	op := q.operator(ch, half)
	q.putword(q.layout.ScratchOperator, uint16(op))
	b := q.mem[at : at+22]
	if q.layout.Name != "LOG" {
		q.write(0x40+op, 63)
	}
	depth := int(q.mem[q.layout.Shadow+0xbd] & 63)
	if q.mem[q.layout.Depth] != 0 {
		depth |= 128
	}
	if q.mem[q.layout.Depth+1] != 0 {
		depth |= 64
	}
	q.write(0xbd, depth)
	flag := 0
	if q.mem[q.layout.Depth+2] != 0 {
		flag = 64
	}
	q.write(8, flag)
	level := (63 - int(b[6]&63)) | (int(b[7]) << 6)
	if q.layout.Name == "LOG" {
		q.write(0x40+op, level)
	}
	connection := 0
	if b[13] == 0 {
		connection = 1
	}
	q.write(0xc0+ch, int(b[10])<<1|connection)
	q.write(0x60+op, int(b[0])<<4|int(b[1]&15))
	q.write(0x80+op, int(b[2])<<4|int(b[3]&15))
	value := int(b[9] & 15)
	for _, pair := range [][2]int{{11, 128}, {12, 64}, {4, 32}, {5, 16}} {
		if b[pair[0]] != 0 {
			value |= pair[1]
		}
	}
	q.write(0x20+op, value)
	q.write(0xe0+op, int(b[8]&3))
	if q.layout.Name != "LOG" {
		q.write(0x40+op, level)
	}
}

func (q *audioSequence) instrument(ch, instrument int) {
	q.channel(ch)
	if instrument < 0 || instrument >= q.layout.InstrumentCount {
		q.err = fmt.Errorf("%s instrument%d outside recognized bank", q.layout.Name, instrument)
		return
	}
	q.putword(q.layout.ScratchChannel, uint16(0xb0+ch))
	q.keyoff(ch)
	q.putword(q.layout.ScratchChannel, uint16(ch))
	q.instrumentHalf(ch, instrument, 0)
	at, target := 0x50+instrument*44, q.layout.NoiseBank+ch*8
	q.mem[target] = q.mem[at+14]
	for _, f := range []int{2, 4, 6} {
		q.putword(target+f, q.word(at+14+f))
	}
	q.instrumentHalf(ch, instrument, 1)
}

func (q *audioSequence) level(ch int) {
	b := q.channel(ch)
	reg := 0x40 + q.operator(ch, 1)
	levelField, offset := 6, 29
	if q.layout.Name == "LOG" {
		levelField, offset = 5, 25
	}
	raw := (int(q.mem[b+levelField]) + int(int8(q.mem[b+offset]))) & 63
	q.write(reg, int(q.mem[q.layout.Shadow+reg]&0xc0)|(63-raw))
}

func (q *audioSequence) frequency(ch int) {
	b := q.channel(ch)
	noteField, offset := 4, 28
	if q.layout.Name == "LOG" {
		noteField, offset = 3, 24
	}
	note := int(q.mem[b+noteField])
	fnum := q.word(q.layout.Pitches+2*(note%12)) + uint16(int16(int8(q.mem[b+offset])))
	q.write(0xa0+ch, int(fnum))
	high := q.mem[q.layout.Shadow+0xb0+ch] & 32
	q.write(0xb0+ch, int(high)|int(fnum>>8)|((note/12)<<2))
}

func (q *audioSequence) frequencySlide(ch int) {
	b := q.channel(ch)
	low, high := q.mem[q.layout.Shadow+0xa0+ch], q.mem[q.layout.Shadow+0xb0+ch]
	v := (uint16(high&31)<<8 | uint16(low)) + uint16(int16(int8(q.mem[b+1])))
	q.write(0xa0+ch, int(v))
	q.write(0xb0+ch, int(high&32)|int(v>>8))
}

func (q *audioSequence) note(ch int) {
	q.level(ch)
	descriptor := q.layout.NoiseBank + ch*8
	if q.mem[descriptor] == 0 {
		q.frequency(ch)
		q.write(0xb0+ch, int(q.mem[q.layout.Shadow+0xb0+ch]|32))
		return
	}
	if byte(ch) == q.mem[q.layout.Owners] {
		q.mem[q.layout.Alternate] = 0
	}
	if byte(ch) == q.mem[q.layout.Owners+1] {
		q.mem[q.layout.Alternate] = 1
	}
	slot := 0
	if q.mem[q.layout.Alternate] != 0 {
		slot = 1
	}
	q.mem[q.layout.Alternate] = byte(1 - slot)
	// The second-slot branch also tests slot0 life in the original. Do not replace
	// this with an independently allocated modern voice or normalize the predicate.
	if q.mem[q.layout.Life] != 0 {
		old := int(q.mem[q.layout.Owners+slot])
		q.write(0xb0+old, int(q.mem[q.layout.Shadow+0xb0+old]&0xdf))
	}
	q.mem[q.layout.Owners+slot] = byte(ch)
	q.mem[q.layout.Life+slot] = q.mem[descriptor]
	q.putword(q.layout.Masks+slot*2, q.word(descriptor+2))
	q.putword(q.layout.Bases+slot*2, q.word(descriptor+4))
	q.putword(q.layout.Deltas+slot*2, q.word(descriptor+6))
	q.setFast(true)
}

func (q *audioSequence) noiseTick() {
	value := q.rng()
	for slot := 0; slot < 2; slot++ {
		if q.mem[q.layout.Life+slot] != 0 {
			v := value
			if slot == 0 {
				v = ^v
			}
			f := (v & q.word(q.layout.Masks+slot*2)) + q.word(q.layout.Bases+slot*2)
			ch := int(q.mem[q.layout.Owners+slot])
			q.write(0xa0+ch, int(f))
			q.write(0xb0+ch, int(f>>8)|32)
		}
	}
}
