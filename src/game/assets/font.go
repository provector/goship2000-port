package assets

import (
	"bytes"
	"fmt"
	"image"
)

// BitmapFont uses prepared source-bit masks, never host font rasterization.
// Top-left coordinates and explicit line placement replace an unknown baseline.
type BitmapFont struct {
	Metrics FontMetrics
	Mask    *image.Paletted
	glyphs  map[byte]FontGlyph
}

func NewBitmapFont(metrics FontMetrics, mask *image.Paletted) (*BitmapFont, error) {
	if mask == nil || metrics.SchemaVersion != 1 || metrics.Height <= 0 || metrics.Height > 64 || metrics.Extra < 0 || metrics.Extra > 64 || metrics.Spacing < 0 || metrics.Spacing > 32 || metrics.First < 0 || metrics.Last > 127 || metrics.Last < metrics.First {
		return nil, fmt.Errorf("invalid bitmap font%d metrics", metrics.Selector)
	}
	if metrics.AtlasSize[0] <= 0 || metrics.AtlasSize[1] <= 0 || metrics.AtlasSize[0] > 4096 || metrics.AtlasSize[1] > 4096 || mask.Stride > 4096 || mask.Stride < metrics.AtlasSize[0] || len(mask.Pix) < mask.Stride*(metrics.AtlasSize[1]-1)+metrics.AtlasSize[0] || mask.Rect != image.Rect(0, 0, metrics.AtlasSize[0], metrics.AtlasSize[1]) || len(metrics.Glyphs) != metrics.Last-metrics.First+1 {
		return nil, fmt.Errorf("font%d atlas/glyph count mismatch", metrics.Selector)
	}
	packed := make([]byte, 0, mask.Rect.Dx()*mask.Rect.Dy())
	for y := 0; y < mask.Rect.Dy(); y++ {
		packed = append(packed, mask.Pix[y*mask.Stride:y*mask.Stride+mask.Rect.Dx()]...)
	}
	if Hash(packed) != metrics.AtlasMaskSHA256 {
		return nil, fmt.Errorf("font%d decoded mask hash mismatch", metrics.Selector)
	}
	for _, pixel := range packed {
		if pixel > 1 {
			return nil, fmt.Errorf("font%d mask contains non-binary index", metrics.Selector)
		}
	}
	font := &BitmapFont{Metrics: metrics, Mask: mask, glyphs: map[byte]FontGlyph{}}
	for _, g := range metrics.Glyphs {
		if g.Rect[0] < 0 || g.Rect[1] < 0 || g.Rect[2] <= 0 || g.Rect[3] <= 0 || g.Rect[2] > metrics.AtlasSize[0] || g.Rect[3] > metrics.AtlasSize[1] || g.Rect[0] > metrics.AtlasSize[0]-g.Rect[2] || g.Rect[1] > metrics.AtlasSize[1]-g.Rect[3] {
			return nil, fmt.Errorf("font%d glyph%d outside atlas bounds", metrics.Selector, g.Byte)
		}
		r := image.Rect(g.Rect[0], g.Rect[1], g.Rect[0]+g.Rect[2], g.Rect[1]+g.Rect[3])
		if g.Byte < metrics.First || g.Byte > metrics.Last || r.Empty() || !r.In(mask.Rect) || g.Rect[3] != metrics.Height+metrics.Extra || g.Width < 0 || g.Advance != g.Width+metrics.Spacing || g.Advance > g.Rect[2] || g.Width > g.Rect[2] {
			return nil, fmt.Errorf("font%d glyph%d has unsupported geometry/advance", metrics.Selector, g.Byte)
		}
		if _, exists := font.glyphs[byte(g.Byte)]; exists {
			return nil, fmt.Errorf("font%d duplicates glyph%d", metrics.Selector, g.Byte)
		}
		font.glyphs[byte(g.Byte)] = g
	}
	return font, nil
}

func beforeNUL(raw []byte) []byte {
	if end := bytes.IndexByte(raw, 0); end >= 0 {
		return raw[:end]
	}
	return raw
}

func (font *BitmapFont) Measure(raw []byte) int {
	width := 0
	for _, b := range beforeNUL(raw) {
		if b&128 != 0 {
			continue
		}
		if glyph, ok := font.glyphs[b]; ok {
			width += glyph.Advance
		}
	}
	return width
}

func (font *BitmapFont) Draw(dst []byte, raw []byte, x, y, foreground, background, flags int) error {
	return font.DrawClipped(dst, raw, x, y, foreground, background, flags, [4]int{0, 0, 320, 200})
}

// Menu text uses an AREA-local clip in addition to the logical framebuffer.
// Advances remain unchanged when pixels fall outside that rectangle.
func (font *BitmapFont) DrawClipped(dst []byte, raw []byte, x, y, foreground, background, flags int, clip [4]int) error {
	if len(dst) != 320*200 {
		return fmt.Errorf("font destination must be 320x200 indices")
	}
	if foreground < 0 || foreground > 255 || background < 0 || background > 255 || flags < 0 || flags > 1 {
		return fmt.Errorf("unsupported font colour/flags")
	}
	raw = beforeNUL(raw)
	rows := font.Metrics.Height
	if flags&1 != 0 {
		rows += font.Metrics.Extra
	}
	for index, b := range raw {
		if b&128 != 0 {
			foreground = int(b & 127)
			continue
		}
		glyph, ok := font.glyphs[b]
		if !ok {
			continue
		}
		columns := glyph.Advance
		// The transparent native path suppresses spacing except after the final
		// source byte; flags1 writes both foreground and background over the advance.
		if flags != 1 && index+1 < len(raw) {
			columns -= font.Metrics.Spacing
		}
		for row := 0; row < rows; row++ {
			dy := y + row
			if dy < 0 || dy >= 200 || dy < clip[1] || dy >= clip[1]+clip[3] {
				continue
			}
			for col := 0; col < columns; col++ {
				dx := x + col
				if dx < 0 || dx >= 320 || dx < clip[0] || dx >= clip[0]+clip[2] {
					continue
				}
				on := font.Mask.ColorIndexAt(glyph.Rect[0]+col, glyph.Rect[1]+row) != 0
				if on {
					dst[dy*320+dx] = byte(foreground)
				} else if flags == 1 {
					dst[dy*320+dx] = byte(background)
				}
			}
		}
		x += glyph.Advance
	}
	return nil
}

func (font *BitmapFont) DrawCentered(dst []byte, raw []byte, y, foreground, background, flags int) error {
	return font.Draw(dst, raw, (320-font.Measure(raw))/2, y, foreground, background, flags)
}

type LetterCommand struct {
	Byte        byte
	Source      [4]int
	Destination [2]int
}

type AtlasGlyphCommand = LetterCommand

// AtlasTextCommands preserves the original one-time backward break calculation.
// It intentionally is not a general word wrapper: the second line can truncate.
func AtlasTextCommands(raw string, x, y, width, lines int) []LetterCommand {
	text := beforeNUL([]byte(raw))
	if width < 8 || width > 320 || lines <= 0 || lines > 200 {
		return nil
	}
	capacity := width/4 - 1
	breakIndex := capacity - 1
	if capacity < len(text) && lines >= 2 {
		for breakIndex != 0 {
			b := text[breakIndex]
			if !asciiLetterOrDigit(b) {
				break
			}
			breakIndex--
		}
		if breakIndex == 0 {
			breakIndex = capacity - 1
		}
	}
	var commands []LetterCommand
	advance := 0
	for i, b := range text {
		if advance >= width {
			break
		}
		// Original signed-byte acceptance excludes bytes with their high bit set.
		if b >= 32 && b <= 127 {
			n := int(b) - 32
			commands = append(commands, LetterCommand{Byte: b, Source: [4]int{4 * (n % 48), 190 + 5*(n/48), 4, 5}, Destination: [2]int{x + advance, y}})
		}
		advance += 4
		if i == breakIndex {
			lines--
			if lines == 0 {
				return commands
			}
			y += 6
			advance = 0
		}
	}
	return commands
}

func asciiLetterOrDigit(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}
