package assets

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Startup frames are complete immutable indexed surfaces. Source coordinates in
// PAN are logical decompressed offsets; SS coordinates are physical file bytes.
type StartupAnimation struct {
	media              *packedFile
	packPath           string
	Name               string         `json:"name"`
	Width              int            `json:"width"`
	Height             int            `json:"height"`
	SourceSHA256       string         `json:"source_sha256"`
	CompressedConsumed int            `json:"compressed_consumed"`
	LogicalBytes       int            `json:"logical_bytes"`
	TimingBasis        string         `json:"timing_basis"`
	PaletteKnown       []bool         `json:"palette_known"`
	Frames             []StartupFrame `json:"-"`
	Prelude            []StartupFrame `json:"-"`
}

type StartupFrame struct {
	digest             string
	Pixels             []byte  `json:"-"`
	PaletteRGB6        []byte  `json:"-"`
	DelayBeforeSeconds float64 `json:"delay_before_seconds"`
	HoldSeconds        float64 `json:"hold_seconds"`
	SourceStart        int     `json:"source_start"`
	SourceEnd          int     `json:"source_end"`
	HoldRaw            uint16  `json:"hold_raw"`
}

const StartupPixels = 320 * 200

const maxStartupLogical = 16 * 1024 * 1024

func startupIdentity(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// The first control word is loaded before the first literal. Its final bit
// refills immediately, before the token's following data bytes are consumed.
type startupBits struct {
	data []byte
	pos  int
	bits uint16
	left int
}

func (r *startupBits) byte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("startup compressed input truncated at byte %#x", r.pos)
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *startupBits) bit() (int, error) {
	if r.left == 0 {
		return 0, fmt.Errorf("startup control word not initialized")
	}
	b := int(r.bits & 1)
	r.bits >>= 1
	r.left--
	if r.left == 0 {
		lo, e := r.byte()
		if e != nil {
			return 0, e
		}
		hi, e := r.byte()
		if e != nil {
			return 0, e
		}
		r.bits = uint16(lo) | uint16(hi)<<8
		r.left = 16
	}
	return b, nil
}

// The bounded PAN variant uses the thirteen-bit, signed backwards-copy form
// confirmed in PLAYER's decoder. Span markers preserve dictionary/control state;
// marker zero ends the stream and marker one merely ends a caller buffer span.
func expandStartupPAN(data []byte) ([]byte, int, error) {
	if len(data) < 2 {
		return nil, 0, fmt.Errorf("PAN missing first control word")
	}
	r := startupBits{data: data, pos: 2, bits: binary.LittleEndian.Uint16(data), left: 16}
	out := make([]byte, 0, len(data)*2)
	for len(out) <= maxStartupLogical {
		v, e := r.bit()
		if e != nil {
			return nil, r.pos, e
		}
		if v != 0 {
			b, e := r.byte()
			if e != nil {
				return nil, r.pos, e
			}
			out = append(out, b)
			continue
		}
		v, e = r.bit()
		if e != nil {
			return nil, r.pos, e
		}
		n, back := 0, 0
		if v == 0 {
			hi, e := r.bit()
			if e != nil {
				return nil, r.pos, e
			}
			lo, e := r.bit()
			if e != nil {
				return nil, r.pos, e
			}
			n = 2 + hi*2 + lo
			b, e := r.byte()
			if e != nil {
				return nil, r.pos, e
			}
			back = int(b) - 256
		} else {
			lo, e := r.byte()
			if e != nil {
				return nil, r.pos, e
			}
			hi, e := r.byte()
			if e != nil {
				return nil, r.pos, e
			}
			back = int(uint16(hi>>3|0xe0)<<8|uint16(lo)) - 65536
			n = int(hi & 7)
			if n == 0 {
				b, e := r.byte()
				if e != nil {
					return nil, r.pos, e
				}
				if b == 0 {
					return out, r.pos, nil
				}
				if b == 1 {
					continue
				}
				n = int(b) + 1
			} else {
				n += 2
			}
		}
		if back >= 0 || len(out)+back < 0 {
			return nil, r.pos, fmt.Errorf("PAN back-reference %d before logical byte %#x", back, len(out))
		}
		if len(out)+n > maxStartupLogical {
			return nil, r.pos, fmt.Errorf("PAN logical output exceeds %d bytes", maxStartupLogical)
		}
		for i := 0; i < n; i++ {
			out = append(out, out[len(out)+back])
		}
	}
	return nil, r.pos, fmt.Errorf("PAN logical output limit")
}

func DecodeStartupPAN(data []byte) (*StartupAnimation, error) {
	if len(data) > maxStartupLogical {
		return nil, fmt.Errorf("PAN source exceeds supported input limit")
	}
	logical, consumed, e := expandStartupPAN(data)
	if e != nil {
		return nil, e
	}
	if len(logical) < 4 || logical[0] != 0x5a || logical[1] == 0 || logical[1] > 60 {
		return nil, fmt.Errorf("PAN unsupported logical header or callback divisor")
	}
	n := int(binary.LittleEndian.Uint16(logical[2:]))
	if n < 1 || n > 1024 {
		return nil, fmt.Errorf("PAN frame count %d outside supported bounds", n)
	}
	p := 4 + 2*n
	if p+768 > len(logical) {
		return nil, fmt.Errorf("PAN truncated frame table/palette")
	}
	palette := append([]byte(nil), logical[p:p+768]...)
	for _, v := range palette {
		if v > 63 {
			return nil, fmt.Errorf("PAN palette component %d exceeds RGB6", v)
		}
	}
	p += 768
	scene := make([]byte, StartupPixels)
	a := &StartupAnimation{Name: "GST.PAN", Width: 320, Height: 200, SourceSHA256: startupIdentity(data), CompressedConsumed: consumed, LogicalBytes: len(logical), TimingBasis: "PLAYER computes 60/header[1] callback ticks per frame; nominal 60 Hz callback cadence, elapsed-time approximation; fades handled separately", PaletteKnown: make([]bool, 256)}
	for i := range a.PaletteKnown {
		a.PaletteKnown[i] = true
	}
	for i := 0; i < n; i++ {
		span := int(binary.LittleEndian.Uint16(logical[4+i*2:]))
		end := p + span
		// The first table word includes header/table/palette, unlike later spans.
		if i == 0 {
			end = span
		}
		if end <= p || end > len(logical) {
			return nil, fmt.Errorf("PAN frame %d invalid logical interval [%#x,%#x)", i, p, end)
		}
		start := p
		if end-p < 4 || string(logical[p:p+4]) != "B\x00\x01\x00" {
			return nil, fmt.Errorf("PAN frame %d missing B/1 header at %#x", i, p)
		}
		p += 4
		used, e := applyPANFrame(scene, logical[p:end])
		if e != nil {
			return nil, fmt.Errorf("PAN frame %d: %w", i, e)
		}
		if used != end-p {
			return nil, fmt.Errorf("PAN frame %d end opcode leaves %d bytes", i, end-p-used)
		}
		p = end
		a.Frames = append(a.Frames, StartupFrame{Pixels: append([]byte(nil), scene...), PaletteRGB6: palette, HoldSeconds: float64(60/int(logical[1])) / 60, SourceStart: start, SourceEnd: end, HoldRaw: uint16(60 / int(logical[1]))})
	}
	if p != len(logical) {
		return nil, fmt.Errorf("PAN frame table leaves %d logical bytes", len(logical)-p)
	}
	// This supported playback profile uses Step18's measured batch150; codec
	// indices stay separate from this hardware-dependent presentation prelude.
	a.Prelude, e = StartupPANFade(a, 150)
	if e != nil {
		return nil, e
	}
	return a, nil
}

func applyPANFrame(scene, cmd []byte) (int, error) {
	p, x := 0, 0
	take := func(n int) ([]byte, error) {
		if n < 0 || n > len(cmd)-p {
			return nil, fmt.Errorf("truncated frame opcode at %#x requesting %d bytes", p, n)
		}
		b := cmd[p : p+n]
		p += n
		return b, nil
	}
	for {
		b, e := take(1)
		if e != nil {
			return p, e
		}
		op := b[0]
		n, kind := 0, 0
		switch {
		case op == 0:
			b, e = take(2)
			if e != nil {
				return p, e
			}
			n = int(b[0])
			kind = 2
		case op == 0x80:
			b, e = take(2)
			if e != nil {
				return p, e
			}
			v := int(binary.LittleEndian.Uint16(b))
			if v == 0 {
				return p, nil
			}
			if v < 0x8000 {
				n = v
				kind = 0
			} else if v < 0xc000 {
				n = v - 0x8000
				kind = 1
			} else {
				n = v - 0xc000
				kind = 3
			}
		case op > 0x80:
			n = int(op) - 0x80
		default:
			n = int(op)
			kind = 1
		}
		if n > len(scene)-x {
			return p, fmt.Errorf("frame operation crosses %d-pixel bank at %d+%d", len(scene), x, n)
		}
		switch kind {
		case 1:
			b, e = take(n)
			if e != nil {
				return p, e
			}
			copy(scene[x:x+n], b)
		case 2:
			for j := 0; j < n; j++ {
				scene[x+j] = b[1]
			}
		case 3:
			b, e = take(1)
			if e != nil {
				return p, e
			}
			for j := 0; j < n; j++ {
				scene[x+j] = b[0]
			}
		}
		x += n
	}
}

func DecodeStartupSS(data []byte) (*StartupAnimation, error) {
	if len(data) < 31 || string(data[:7]) != "MADSS1\x1a" {
		return nil, fmt.Errorf("SS unsupported signature/header")
	}
	n := int(binary.LittleEndian.Uint16(data[20:]))
	if n < 1 || n > 1024 {
		return nil, fmt.Errorf("SS record count %d outside supported bounds", n)
	}
	paletteStart := 31 + n*14
	if paletteStart >= len(data) {
		return nil, fmt.Errorf("SS truncated record table")
	}
	count := int(data[paletteStart])
	if count < 1 || count > 225 || 1+count*4 > len(data)-paletteStart {
		return nil, fmt.Errorf("SS invalid palette extent")
	}
	// LABS loads this type-4 logo at palette base31. Other MADSS variants are
	// intentionally unsupported rather than being assigned the logo's palette.
	if data[26] != 4 {
		return nil, fmt.Errorf("SS unsupported palette/series type %d", data[26])
	}
	palette := make([]byte, 768)
	known := make([]bool, 256)
	known[0] = true
	for i := 0; i < count; i++ {
		for c := 0; c < 3; c++ {
			v := data[paletteStart+1+i*4+c]
			if v > 63 {
				return nil, fmt.Errorf("SS palette RGB6 component exceeds63")
			}
			palette[(31+i)*3+c] = v
		}
		known[31+i] = true
	}
	pixelsStart := paletteStart + 1 + count*4
	scene := make([]byte, StartupPixels)
	a := &StartupAnimation{Name: "LABSLOGO.SS", Width: 320, Height: 200, SourceSHA256: startupIdentity(data), CompressedConsumed: len(data), LogicalBytes: len(data), TimingBasis: "LABS adds each record hold to a cumulative BIOS tick deadline before drawing; nominal 1193182/65536 Hz, no DOS busy waits", PaletteKnown: known}
	previous := 0
	for i := 0; i < n; i++ {
		r := data[31+i*14 : 31+(i+1)*14]
		off := int(binary.LittleEndian.Uint32(r))
		end := len(data) - pixelsStart
		if i+1 < n {
			end = int(binary.LittleEndian.Uint32(data[31+(i+1)*14:]))
		}
		if off < previous || end < off || end > len(data)-pixelsStart {
			return nil, fmt.Errorf("SS record %d invalid payload [%#x,%#x)", i, off, end)
		}
		previous = off
		x := int(binary.LittleEndian.Uint16(r[6:]))
		y := int(binary.LittleEndian.Uint16(r[8:]))
		w := int(binary.LittleEndian.Uint16(r[10:])) + 1
		h := int(binary.LittleEndian.Uint16(r[12:])) + 1
		if x >= 320 || y >= 200 || w > 320-x || h > 200-y {
			return nil, fmt.Errorf("SS record %d rectangle outside320x200", i)
		}
		used, e := applySSFrame(scene, data[pixelsStart+off:pixelsStart+end], x, y, w, h, count)
		if e != nil {
			return nil, fmt.Errorf("SS record %d: %w", i, e)
		}
		if used != end-off {
			return nil, fmt.Errorf("SS record %d has %d unused bytes", i, end-off-used)
		}
		hold := binary.LittleEndian.Uint16(r[4:])
		a.Frames = append(a.Frames, StartupFrame{Pixels: append([]byte(nil), scene...), PaletteRGB6: palette, DelayBeforeSeconds: float64(hold) / (1193182.0 / 65536), SourceStart: pixelsStart + off, SourceEnd: pixelsStart + end, HoldRaw: hold})
	}
	return a, nil
}

// FA is literal-line mode, FD introduces a count/value run and FC leaves the
// previous destination pixel intact. Every other line mode uses count/value
// pairs (F9 in this asset). FE advances a row, FF ends the sprite.
func applySSFrame(scene, cmd []byte, left, top, width, height, colours int) (int, error) {
	p, x, y := 0, 0, 0
	take := func() (byte, error) {
		if p >= len(cmd) {
			return 0, fmt.Errorf("truncated SS command at %#x", p)
		}
		v := cmd[p]
		p++
		return v, nil
	}
	write := func(n int, c byte) error {
		if n == 0 {
			n = 256
		}
		if y >= height || n > width-x {
			return fmt.Errorf("SS run exceeds rectangle at row%d,x%d,count%d", y, x, n)
		}
		if c != 0xfc {
			if c == 0 || int(c) > colours {
				return fmt.Errorf("SS colour%d outside source palette", c)
			}
			for j := 0; j < n; j++ {
				// Source palette colours are one-based; LABS adds base-1.
				scene[(top+y)*320+left+x+j] = c + 30
			}
		}
		x += n
		return nil
	}
	for {
		mode, e := take()
		if e != nil {
			return p, e
		}
		if mode == 0xff {
			return p, nil
		}
		if mode == 0xfe {
			y++
			x = 0
			continue
		}
		for {
			v, e := take()
			if e != nil {
				return p, e
			}
			if v == 0xfe {
				y++
				x = 0
				break
			}
			n, c := 1, v
			if mode != 0xfa || v == 0xfd {
				v, e = take()
				if e != nil {
					return p, e
				}
				if mode == 0xfa {
					n = int(v)
					c, e = take()
					if e != nil {
						return p, e
					}
				} else {
					n = int(c)
					c = v
				}
			}
			if e = write(n, c); e != nil {
				return p, e
			}
		}
	}
}

// StartupPANFade reconstructs the actual round-robin DAC update. Batch size is
// calibrated by original hardware timing;150 is the saved Step18 debugger
// environment's measured value, not a universal codec constant. Playback clock
// duration remains nominal60Hz. No captured pixel or palette is used here.
func StartupPANFade(a *StartupAnimation, batch int) ([]StartupFrame, error) {
	if a == nil || len(a.Frames) == 0 || len(a.Frames[0].PaletteRGB6) != 768 {
		return nil, fmt.Errorf("PAN fade needs a decoded first frame/palette")
	}
	if batch < 1 || batch > 256 {
		return nil, fmt.Errorf("PAN fade batch%d outside1..256", batch)
	}
	const callbacks = 300
	denominator := (batch * callbacks) >> 8
	denominator--
	if denominator < 1 {
		return nil, fmt.Errorf("PAN fade batch too small for supported five-second fade")
	}
	step := 65536 / denominator
	if step > 65535 {
		return nil, fmt.Errorf("PAN fade increment overflows16bits")
	}
	index := (-batch) & 255
	factor := step
	source := a.Frames[0].PaletteRGB6
	installed := make([]byte, 768)
	frame := func(palette []byte) StartupFrame {
		return StartupFrame{Pixels: a.Frames[0].Pixels, PaletteRGB6: append([]byte(nil), palette...), HoldSeconds: 1.0 / 60, HoldRaw: 1, SourceStart: a.Frames[0].SourceStart, SourceEnd: a.Frames[0].SourceEnd}
	}
	initial := frame(installed)
	initial.HoldSeconds = 0 // Black at the first presentation boundary, before callback0.
	initial.HoldRaw = 0
	frames := []StartupFrame{initial}
	next := func() []byte {
		index = (index + batch) & 255
		pending := make([]byte, batch*3)
		for j := 0; j < batch; j++ {
			entry := (index + j) & 255
			for k := 0; k < 3; k++ {
				pending[j*3+k] = byte((int(source[entry*3+k])*factor + 32768) >> 16)
			}
			if entry == 255 {
				factor += step
				if factor > 65535 {
					factor = 65535
				}
			}
		}
		return pending
	}
	pending := next()
	for tick := 0; tick < callbacks; tick++ {
		for j := 0; j < batch; j++ {
			entry := (index + j) & 255
			copy(installed[entry*3:entry*3+3], pending[j*3:j*3+3])
		}
		frames = append(frames, frame(installed))
		pending = next()
	}
	if !equalStartupPalette(installed, source) {
		return nil, fmt.Errorf("PAN fade did not reach complete source palette")
	}
	return frames, nil
}

func equalStartupPalette(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
