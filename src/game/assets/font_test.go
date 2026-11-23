package assets

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

func testBitmapFont(t *testing.T) *BitmapFont {
	t.Helper()
	mask := image.NewPaletted(image.Rect(0, 0, 8, 3), color.Palette{color.Black, color.White})
	mask.SetColorIndex(0, 0, 1)
	mask.SetColorIndex(1, 1, 1)
	mask.SetColorIndex(0, 2, 1)
	metrics := FontMetrics{SchemaVersion: 1, Selector: 5, AtlasSize: [2]int{8, 3}, AtlasMaskSHA256: Hash(mask.Pix), First: 65, Last: 65, Height: 2, Spacing: 1, Extra: 1, Glyphs: []FontGlyph{{Byte: 65, Rect: [4]int{0, 0, 8, 3}, Width: 2, Advance: 3}}}
	font, err := NewBitmapFont(metrics, mask)
	if err != nil {
		t.Fatal(err)
	}
	return font
}

func TestBitmapFontMaskRecolourClippingAndControls(t *testing.T) {
	font := testBitmapFont(t)
	dst := make([]byte, 64000)
	for i := range dst {
		dst[i] = 9
	}
	if err := font.Draw(dst, []byte{'A', 128 | 12, 'A', 0, 'A'}, -1, 198, 7, 0, 0); err != nil {
		t.Fatal(err)
	}
	if dst[199*320] != 7 || dst[198*320+2] != 12 || dst[198*320+3] != 9 {
		t.Fatal("transparent mask, clipping or in-band foreground control differs")
	}
	if font.Measure([]byte{'A', 128 | 12, 'A', 0, 'A'}) != 6 {
		t.Fatal("measure must ignore colour controls and stop at NUL")
	}
}

func TestBitmapFontOpaqueExtraRowAndTransparentSpacing(t *testing.T) {
	font := testBitmapFont(t)
	dst := make([]byte, 64000)
	for i := range dst {
		dst[i] = 9
	}
	if err := font.Draw(dst, []byte{'A'}, 0, 0, 7, 2, 1); err != nil {
		t.Fatal(err)
	}
	if dst[0] != 7 || dst[1] != 2 || dst[2] != 2 || dst[2*320] != 7 {
		t.Fatal("opaque drawing must recolour background and optional row")
	}
	if err := font.Draw(dst, []byte{'A'}, 0, 0, 7, 2, 2); err == nil {
		t.Fatal("unsupported flags accepted")
	}
}

func TestBitmapFontRejectsMalformedMetrics(t *testing.T) {
	font := testBitmapFont(t)
	bad := font.Metrics
	bad.Glyphs = append([]FontGlyph(nil), bad.Glyphs...)
	bad.Glyphs[0].Advance = 100
	if _, err := NewBitmapFont(bad, font.Mask); err == nil {
		t.Fatal("oversized glyph accepted")
	}
	bad = font.Metrics
	bad.AtlasMaskSHA256 = "incorrect"
	if _, err := NewBitmapFont(bad, font.Mask); err == nil {
		t.Fatal("mask mismatch accepted")
	}
}

func TestBitmapFontMenuClipPreservesAdjacentRows(t *testing.T) {
	font := testBitmapFont(t)
	dst := make([]byte, 64000)
	for i := range dst {
		dst[i] = 9
	}
	// Only the middle row/column of an opaque glyph falls inside this slot.
	if err := font.DrawClipped(dst, []byte{'A'}, 10, 10, 7, 2, 1, [4]int{11, 11, 1, 1}); err != nil {
		t.Fatal(err)
	}
	for i, value := range dst {
		want := byte(9)
		if i == 11*320+11 {
			want = 7
		}
		if value != want {
			t.Fatalf("menu clip changed pixel %d to %d, want %d", i, value, want)
		}
	}
}

func TestAtlasTextSingleBackwardBreakAndRejectedBytes(t *testing.T) {
	commands := AtlasTextCommands("Test Squadron Name", 169, 52, 48, 2)
	if len(commands) != 17 {
		t.Fatalf("got %d commands, want bounded 17", len(commands))
	}
	var first, second []byte
	for _, c := range commands {
		if c.Destination[1] == 52 {
			first = append(first, c.Byte)
		} else if c.Destination[1] == 58 {
			second = append(second, c.Byte)
		} else {
			t.Fatal("unexpected line")
		}
	}
	if string(first) != "Test " || string(second) != "Squadron Nam" {
		t.Fatalf("lines %q / %q", first, second)
	}
	commands = AtlasTextCommands("A\xffB", 0, 0, 60, 1)
	if len(commands) != 2 || commands[1].Destination[0] != 8 {
		t.Fatal("rejected byte must still advance")
	}
	if !reflect.DeepEqual(commands[0].Source, [4]int{132, 190, 4, 5}) {
		t.Fatal("first-half glyph mapping differs")
	}
	commands = AtlasTextCommands("a", 0, 0, 60, 1)
	if commands[0].Source != [4]int{68, 195, 4, 5} {
		t.Fatal("second-half glyph mapping differs")
	}
	if got := AtlasTextCommands("long", 0, 0, 4, 1); len(got) != 0 {
		t.Fatal("unsupported text width accepted")
	}
}
