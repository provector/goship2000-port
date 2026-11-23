package cockpit

import (
	"fmt"
	"strings"
)

type Rect struct{ Left, Top, Right, Bottom int }

var Full = Rect{0, 0, 319, 199}

func pixel(dst []byte, x, y int, c byte, clip Rect) {
	if x >= 0 && x < 320 && y >= 0 && y < 200 && x >= clip.Left && x <= clip.Right && y >= clip.Top && y <= clip.Bottom {
		PutLogical(dst, y*320+x, c)
	}
}

func Line(dst []byte, x0, y0, x1, y1 int, c byte, clip Rect) {
	// Selected-VGA0729 includes both endpoints and steps the minor axis only
	// when SUB borrows. An exact half-major tie keeps the current minor axis.
	// World near-plane clipping is separate; HUD vectors clip emitted pixels.
	if !LogicalTarget(dst) {
		return
	}
	if x0 > x1 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
	}
	dx, dy := x1-x0, y1-y0
	vertical := 1
	if dy < 0 {
		vertical = -1
		dy = -dy
	}
	major, minor, ax, ay, bx, by := dx, dy, 1, 0, 0, vertical
	if dx < dy {
		major, minor, ax, ay, bx, by = dy, dx, bx, by, ax, ay
	}
	if major > 8192 {
		return
	}
	err := major / 2
	x, y := x0, y0
	for i := 0; i <= major; i++ {
		pixel(dst, x, y, c, clip)
		if i == major {
			break
		}
		x += ax
		y += ay
		err -= minor
		if err < 0 {
			err += major
			x += bx
			y += by
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
func fill(dst []byte, x, y, w, h int, c byte, clip Rect) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			pixel(dst, xx, yy, c, clip)
		}
	}
}

func blit(dst []byte, img Image, sx, sy, w, h, x, y int, zeroSkip bool, clip Rect) error {
	if sx < 0 || sy < 0 || w < 0 || h < 0 || sx+w > img.Width || sy+h > img.Height || len(img.Pixels) != img.Width*img.Height {
		return fmt.Errorf("HUD source rectangle outside atlas")
	}
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			v := img.Pixels[(sy+row)*img.Width+sx+col]
			if !zeroSkip || v != 0 {
				pixel(dst, x+col, y+row, v, clip)
			}
		}
	}
	return nil
}

func (a *Assets) Sprite(dst []byte, name string, x, y int, clip Rect) error {
	i, ok := a.Sprites[name]
	if !ok {
		return fmt.Errorf("HUD sprite %q missing", name)
	}
	return blit(dst, i, 0, 0, i.Width, i.Height, x, y, true, clip)
}

func (a *Assets) Number(dst []byte, x, y int, value int16, style uint16) error {
	n := int(value)
	if n < 0 {
		n = -n
	}
	digits := int(style & 7)
	if digits < 1 || digits > 4 {
		return fmt.Errorf("unsupported HUD digit count %d", digits)
	}
	limit := 1
	for j := 0; j < digits; j++ {
		limit *= 10
	}
	if value == -32768 || n >= limit {
		return fmt.Errorf("HUD value %d exceeds %d-digit source glyph domain", value, digits)
	}
	small := style&0x100 != 0
	name := "digits-opaque"
	w := 6
	if small {
		name = "digits-zero-mask"
		w = 4
	}
	img := a.Atlases[name]
	delta := -1
	if !small && style&0x200 == 0 {
		delta++
	}
	if style&0x400 == 0 {
		delta++
	}
	div := limit / 10
	for j := 0; j < digits; j++ {
		digit := n / div
		n %= div
		div /= 10
		if e := blit(dst, img, 0, digit*6, w, 6, x, y, small, Full); e != nil {
			return e
		}
		x += w + delta
	}
	return nil
}

func (a *Assets) textWidth(s string, style uint16) (int, error) {
	n := 0
	for _, ch := range []byte(s) {
		if ch < 32 || int(ch) > a.Font.Last {
			return 0, fmt.Errorf("unsupported HUD text byte %02x", ch)
		}
		if style&2 != 0 {
			n += int(a.Font.Advances[int(ch)-a.Font.First])
		} else {
			n += a.Font.Width
		}
	}
	return n, nil
}

func (a *Assets) Text(dst []byte, text string, x, y int, colour byte, style uint16, clip Rect) error {
	// The original wrapper edits its argument. Native text uses a private copy
	// so drawing cannot mutate prepared strings or authoritative mission data.
	text = strings.SplitN(text, "\x00", 2)[0]
	lines := strings.Split(text, "\n")
	if style&0x100 != 0 {
		lines = nil
		for _, paragraph := range strings.Split(text, "\n") {
			line := ""
			for _, word := range strings.Split(paragraph, " ") {
				candidate := word
				if line != "" {
					candidate = line + " " + word
				}
				width, e := a.textWidth(candidate, style)
				if e != nil {
					return e
				}
				if x+width > clip.Right+1 && line != "" {
					lines = append(lines, line)
					line = word
				} else {
					line = candidate
				}
			}
			if width, e := a.textWidth(line, style); e != nil {
				return e
			} else if width > clip.Right-clip.Left+1 {
				return fmt.Errorf("unbreakable HUD text exceeds source wrap domain")
			}
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		width, e := a.textWidth(line, style)
		if e != nil {
			return e
		}
		xx := x
		if style&0x200 != 0 {
			xx = (clip.Right-width-clip.Left)/2 + clip.Left
		}
		if style&0x800 != 0 {
			xx = x - width/2 + 1
		}
		for _, ch := range []byte(line) {
			idx := int(ch) - a.Font.First
			advance := a.Font.Width
			if style&2 != 0 {
				advance = int(a.Font.Advances[idx])
			}
			for row := 0; row < a.Font.Height; row++ {
				bits := a.Font.Rows[idx*a.Font.Height+row]
				for col := 0; col < a.Font.Width; col++ {
					if bits&(0x80>>col) == 0 {
						continue
					}
					dx, dy := xx+col, y+row
					if style&0x30 != 0 {
						sx, sy := dx, dy
						if style&0x10 != 0 {
							sy++
						}
						if style&0x20 != 0 {
							sx++
						}
						pixel(dst, sx, sy, 0, clip)
					}
					if style&0x80 != 0 {
						dy++
					}
					if style&0x40 != 0 {
						dx++
					}
					pixel(dst, dx, dy, colour, clip)
				}
			}
			xx += advance
		}
		y += a.Font.Height
		if style&0xf0 != 0 {
			y++
		}
		x = clip.Left
	}
	return nil
}

func exactReplace(dst []byte, x, y, w, h int, match, to byte) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if xx >= 0 && xx < 320 && yy >= 0 && yy < 200 {
				p := yy*320 + xx
				if dst[p] == match {
					PutLogical(dst, p, to)
				}
			}
		}
	}
}
