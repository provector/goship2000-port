package assets

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func syntheticFont() []byte {
	// Same source grammar as Step13's independent focused glyph fixture.
	b := make([]byte, 17)
	binary.LittleEndian.PutUint16(b, 1)
	binary.LittleEndian.PutUint16(b[2:], 13)
	b[4] = 2
	copy(b[5:13], []byte{65, 65, 2, 0, 2, 1, 0, 0})
	copy(b[13:], []byte{0x80, 0, 0x40, 0})
	return b
}

func TestOfficeFontMSBAdvanceAndMaskPNG(t *testing.T) {
	m, atlas, err := parseOfficeFont(syntheticFont(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Height != 2 || m.Glyphs[0].Advance != 3 || m.RowStride != 2 {
		t.Fatalf("wrong metrics: %+v", m)
	}
	if atlas.ColorIndexAt(0, 0) != 1 || atlas.ColorIndexAt(1, 0) != 0 || atlas.ColorIndexAt(1, 1) != 1 || atlas.ColorIndexAt(0, 1) != 0 {
		t.Fatal("glyph rows are not MSB-first")
	}
	b, err := PngBytes(atlas)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Bounds() != atlas.Bounds() {
		t.Fatal("PNG bounds changed")
	}
	_, _, _, a0 := reopened.At(1, 0).RGBA()
	_, _, _, a1 := reopened.At(0, 0).RGBA()
	if a0 != 0 || a1 != 65535 {
		t.Fatal("font mask transparency changed")
	}
}

func TestOfficeFontExtraRowsAndBounds(t *testing.T) {
	b := syntheticFont()
	b[11] = 1
	if _, _, err := parseOfficeFont(b, 1); err == nil {
		t.Fatal("missing conditional extra row accepted")
	}
	b = append(b, 0x20, 0)
	m, atlas, err := parseOfficeFont(b, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.BitmapRange != [2]int{13, 19} || atlas.ColorIndexAt(2, 2) != 1 {
		t.Fatal("extra source row was lost")
	}
	for _, bad := range [][]byte{nil, {1, 0}, b[:12]} {
		if _, _, err := parseOfficeFont(bad, 1); err == nil {
			t.Fatal("truncated font accepted")
		}
	}
	if _, _, err := parseOfficeFont(b, 0); err == nil {
		t.Fatal("selector0 accepted")
	}
	if _, _, err := parseOfficeFont(b, 2); err == nil {
		t.Fatal("out of range selector accepted")
	}
}

func TestOfficeFontStoredCellAndAdvanceRemainDistinct(t *testing.T) {
	b := syntheticFont()
	b[7] = 1
	b[4] = 8
	m, _, err := parseOfficeFont(b, 1)
	if err != nil {
		t.Fatal(err)
	}
	if m.Glyphs[0].Rect[2] != 8 || m.Glyphs[0].Advance != 9 || !m.Glyphs[0].AdvanceBeyondCell {
		t.Fatal("one-byte storage and native advance were conflated")
	}
}

func TestOfficeFontIdentityGate(t *testing.T) {
	r := &Resource{Payload: syntheticFont()}
	accepted, err := exportOfficeFonts(nil, r, nil)
	if accepted || err != nil {
		t.Fatal("unrecognized font identity was converted")
	}
}
