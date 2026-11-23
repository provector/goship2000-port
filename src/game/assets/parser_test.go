package assets

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func catalogFixture() []byte {
	b := make([]byte, 30)
	binary.LittleEndian.PutUint16(b, 1)
	copy(b[2:], "ONE.PIC")
	binary.LittleEndian.PutUint32(b[18:], 4)
	binary.LittleEndian.PutUint32(b[22:], 26)
	copy(b[26:], []byte{1, 2, 3, 4})
	return b
}

func TestDirectoryBoundsAndRawFields(t *testing.T) {
	b := catalogFixture()
	entries, err := parseDirectory(b)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	e := entries[0]
	if e.DirectoryOffset != 2 || e.Offset != 26 || e.End != 30 || e.Length != 4 || e.NameHex != "4f4e452e5049430000000000" {
		t.Fatalf("raw fields lost: %+v", e)
	}
	for _, n := range []int{0, 1, 2, 25, 29} {
		if _, err := parseDirectory(b[:n]); err == nil {
			t.Fatalf("accepted truncated length%d", n)
		}
	}
	for _, offset := range []uint32{0, 25, 31, 0xffffffff} {
		bad := bytes.Clone(b)
		binary.LittleEndian.PutUint32(bad[22:], offset)
		if _, err := parseDirectory(bad); err == nil {
			t.Fatalf("accepted offset%d", offset)
		}
	}
}

func TestDirectorySharedRangesArePreserved(t *testing.T) {
	b := make([]byte, 54)
	binary.LittleEndian.PutUint16(b, 2)
	for i := 0; i < 2; i++ {
		pos := 2 + 24*i
		copy(b[pos:], "SAME")
		binary.LittleEndian.PutUint32(b[pos+16:], 4)
		binary.LittleEndian.PutUint32(b[pos+20:], 50)
	}
	entries, err := parseDirectory(b)
	if err != nil || len(entries) != 2 || entries[0].Offset != entries[1].Offset {
		t.Fatal(entries, err)
	}
}

func TestNameEscapingAndCollisions(t *testing.T) {
	cases := []struct {
		raw  []byte
		want string
	}{
		{[]byte("BASE.PIC"), "BASE.PIC"}, {[]byte("..\\ABC/./X"), "%2E%2E/ABC/%2E/X"},
		{[]byte("/CON/x. "), "%00/%00CON/x.%20"}, {[]byte{0xff, '%', '~'}, "%FF%25%7E"},
	}
	for _, c := range cases {
		if got := escapedPath(c.raw); got != c.want {
			t.Fatalf("%x: got%s want%s", c.raw, got, c.want)
		}
	}
	a := &pathAllocator{}
	one := a.reserve("CAT/BASE.PIC")
	two := a.reserve("CAT/base.pic")
	if one == two || !strings.HasPrefix(two, "~entry-") {
		t.Fatal(one, two)
	}
	a.reserve("CAT/BASE.PIC.png")
	if a.reserve("CAT/BASE.PIC.png") == "CAT/BASE.PIC.png" {
		t.Fatal("generated conversion overwrites raw member")
	}
	a.reserve("A")
	if a.reserve("A/B") == "A/B" {
		t.Fatal("file/directory prefix conflict")
	}
}

// Independent fixture bit writer, not shared with decoder operations. Explicit
// fixture padding is stored data; no game/expected pixels seed the decoder.
func codeFixture(codes []int, widths []int, w, h, maxBits int) []byte {
	var packed []byte
	bit := 0
	for i, c := range codes {
		n := 9
		if widths != nil {
			n = widths[i]
		}
		for k := 0; k < n; k++ {
			if bit/8 >= len(packed) {
				packed = append(packed, 0)
			}
			if c&(1<<k) != 0 {
				packed[bit/8] |= 1 << uint(bit%8)
			}
			bit++
		}
	}
	if len(packed)%2 == 0 {
		packed = append(packed, 0)
	}
	b := make([]byte, 9)
	copy(b, "X0")
	binary.LittleEndian.PutUint16(b[2:], uint16(5+len(packed)))
	binary.LittleEndian.PutUint16(b[4:], uint16(w))
	binary.LittleEndian.PutUint16(b[6:], uint16(h))
	b[8] = byte(maxBits)
	return append(b, packed...)
}

func TestEstablishedCodecVectors(t *testing.T) {
	cases := []struct {
		name  string
		codes []int
		w, h  int
		want  []byte
	}{
		{"bit order", []int{1, 2, 3, 4}, 4, 1, []byte{1, 2, 3, 4}},
		{"next code", []int{65, 257, 258}, 6, 1, bytes.Repeat([]byte{'A'}, 6)},
		{"dictionary across rows", []int{65, 257}, 1, 3, []byte("AAA")},
		{"repeat across rows", []int{7, 0x90, 4}, 2, 2, bytes.Repeat([]byte{7}, 4)},
		{"literal90", []int{0x90, 0}, 1, 1, []byte{0x90}},
		{"repeat underflow", []int{7, 0x90, 1}, 257, 1, bytes.Repeat([]byte{7}, 257)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := decodePicture(codeFixture(c.codes, nil, c.w, c.h, 11))
			if err != nil || !bytes.Equal(got, c.want) {
				t.Fatal(got, err)
			}
		})
	}
}

func TestCodeWidthGrowthAndReset(t *testing.T) {
	codes := make([]int, 260)
	widths := make([]int, 260)
	for i := range codes {
		codes[i] = 1
		widths[i] = 9
		if i >= 256 {
			codes[i] = 65 + i - 256
			widths[i] = 10
		}
	}
	got, stats, err := decodePicture(codeFixture(codes, widths, 260, 1, 11))
	if err != nil || stats.CodeWidth != 10 {
		t.Fatal(stats, err)
	}
	for i, v := range got {
		if int(v) != codes[i] {
			t.Fatal(i, v)
		}
	}
	got, stats, err = decodePicture(codeFixture(codes, nil, 260, 1, 9))
	if err != nil || stats.Resets != 1 || len(got) != 260 || got[256] != 65 {
		t.Fatal(stats, err)
	}
}

func TestUnknownDictionaryAndBoundsFailures(t *testing.T) {
	for _, codes := range [][]int{{256}, {65, 256}, {1, 300}} {
		if _, _, err := decodePicture(codeFixture(codes, nil, 2, 1, 11)); err == nil {
			t.Fatal("accepted unknown/invalid code", codes)
		}
	}
	raw := codeFixture([]int{1, 2, 3}, nil, 3, 1, 11)
	for n := 0; n < len(raw); n++ {
		if _, _, err := decodePicture(raw[:n]); err == nil {
			t.Fatal("accepted truncation", n)
		}
	}
	bad := bytes.Clone(raw)
	copy(bad, "X1")
	if _, _, err := decodePicture(bad); err == nil {
		t.Fatal("accepted X1")
	}
	bad = bytes.Clone(raw)
	binary.LittleEndian.PutUint16(bad[4:], 65535)
	binary.LittleEndian.PutUint16(bad[6:], 65535)
	if _, _, err := decodePicture(bad); err == nil {
		t.Fatal("accepted excessive dimensions")
	}
}

func TestBackingBytePrefetchNeverInventedOrConsumed(t *testing.T) {
	// Eleven-byte X0: first packed byte plus one final byte; original LE16
	// refill needs a real adjacent byte even though that byte's bits are unused.
	raw := codeFixture([]int{5}, nil, 1, 1, 11)
	raw = raw[:11]
	binary.LittleEndian.PutUint16(raw[2:], 7)
	if _, _, err := decodePicture(raw); err == nil || !strings.Contains(err.Error(), "original backing byte") {
		t.Fatal(err)
	}
	for _, adjacent := range []byte{0, 0x58, 255} {
		got, s, err := decodePicture(append(bytes.Clone(raw), adjacent))
		if err != nil || !bytes.Equal(got, []byte{5}) || s.CodeBits != 9 || s.Prefetches != 1 {
			t.Fatal(got, s, err)
		}
	}
	d, err := newPictureDecoder(append(bytes.Clone(raw), 255))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = d.code()
	if _, err = d.code(); err == nil {
		t.Fatal("accepted outside bits")
	}
}

func TestPaletteRangesAndEmptyImages(t *testing.T) {
	raw := []byte{'M', '0', 5, 0, 7, 7, 1, 2, 63, 'X', '0', 4, 0, 0, 0, 0, 0}
	palettes, pos, end, err := pictureHeader(raw)
	if err != nil || len(palettes) != 1 || pos != 9 || end != len(raw) {
		t.Fatal(palettes, pos, end, err)
	}
	pal := palettes[0]
	if pal.First != 7 || pal.Last != 7 || !bytes.Equal(pal.RGB6, []byte{1, 2, 63}) {
		t.Fatal(pal)
	}
	r, g, b, a := paletteColors(&pal)[7].RGBA()
	if r != 4*257 || g != 8*257 || b != 65535 || a != 65535 {
		t.Fatal(r, g, b, a)
	}
	for _, bad := range [][]byte{raw[:5], raw[:8], append([]byte{'M', '0', 255, 255}, raw[4:]...)} {
		if _, _, _, err := pictureHeader(bad); err == nil {
			t.Fatal("accepted bad palette")
		}
	}
	bad := bytes.Clone(raw)
	bad[8] = 64
	if _, _, _, err := pictureHeader(bad); err == nil {
		t.Fatal("accepted >RGB6")
	}
}

func TestExtractionAndGeneratedNameCollision(t *testing.T) {
	in := t.TempDir()
	raw := catalogFixture()
	if err := os.WriteFile(filepath.Join(in, "GS2000.DAT"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "BASE.PIC.png"), []byte("opaque"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "new")
	m, err := prepareAssets(in, out)
	if err != nil || m.Summary.Entries != 1 || m.Summary.LooseCopied != 1 {
		t.Fatal(m, err)
	}
	got, err := os.ReadFile(filepath.Join(out, "GS2000.DAT", "ONE.PIC"))
	if err != nil || !bytes.Equal(got, raw[26:]) {
		t.Fatal(got, err)
	}
	if _, err = prepareAssets(in, out); err == nil {
		t.Fatal("overwrote output")
	}
	if _, _, err = ValidatePaths(in, filepath.Join(in, "new")); err == nil {
		t.Fatal("allowed input overwrite")
	}
	// A malformed archive is fatal before any output directory is created.
	if err = os.WriteFile(filepath.Join(in, "GS2000.DAT"), raw[:25], 0644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "new")
	if _, err = prepareAssets(in, other); err == nil {
		t.Fatal("accepted malformed archive")
	}
	if _, err = os.Stat(other); !os.IsNotExist(err) {
		t.Fatal("created output before preflight", err)
	}
}

func TestPNGIndicesStayOpaque(t *testing.T) {
	imageBytes := codeFixture([]int{0, 1, 2, 0}, nil, 2, 2, 11)
	in := t.TempDir()
	if err := os.WriteFile(filepath.Join(in, "ONE.PIC"), imageBytes, 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "new")
	m, err := prepareAssets(in, out)
	if err != nil || m.Summary.Pictures != 1 {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "loose", "ONE.PIC.diagnostic-grayscale.png"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 {
		t.Fatal(decoded.Bounds())
	}
	_, _, _, alpha := decoded.At(0, 0).RGBA()
	if alpha != 65535 {
		t.Fatal("index0 became transparent")
	}
}

func TestPartialPaletteKeepsUnspecifiedEntriesDistinct(t *testing.T) {
	in := t.TempDir()
	raw := []byte{'M', '0', 5, 0, 7, 7, 0, 0, 0, 'X', '0', 4, 0, 0, 0, 0, 0}
	if err := os.WriteFile(filepath.Join(in, "ONE.PAL"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "new")
	m, err := prepareAssets(in, out)
	if err != nil || m.Summary.Palettes != 1 {
		t.Fatal(m, err)
	}
	data, err := os.ReadFile(filepath.Join(out, "loose", "ONE.PAL.json"))
	if err != nil || !bytes.Contains(data, []byte(`"first_index": 7`)) || !bytes.Contains(data, []byte("unspecified, not black")) {
		t.Fatal(string(data), err)
	}
	rgb, err := os.ReadFile(filepath.Join(out, "loose", "ONE.PAL.rgb6"))
	if err != nil || !bytes.Equal(rgb, []byte{0, 0, 0}) {
		t.Fatal(rgb, err)
	}
}

func TestConversionFailureDoesNotLosePayloads(t *testing.T) {
	in := t.TempDir()
	bad := codeFixture([]int{65, 256}, nil, 2, 1, 11)
	if err := os.WriteFile(filepath.Join(in, "BAD.PIC"), bad, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "GOOD.PIC"), codeFixture([]int{1}, nil, 1, 1, 11), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "new")
	m, err := prepareAssets(in, out)
	if err != nil || m.Summary.ConversionFailures != 1 || m.Summary.Pictures != 1 || m.Summary.LooseCopied != 2 {
		t.Fatal(m, err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "loose", "BAD.PIC"))
	if err != nil || !bytes.Equal(raw, bad) {
		t.Fatal(raw, err)
	}
	if _, err = os.Stat(filepath.Join(out, "loose", "BAD.PIC.indices")); !os.IsNotExist(err) {
		t.Fatal("partial indices exported", err)
	}
}

func TestSymlinkOutputCannotEnterProtectedInput(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "input")
	if err := os.Mkdir(in, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(in, link); err != nil {
		t.Skip(err)
	}
	if _, _, err := ValidatePaths(in, filepath.Join(link, "output")); err == nil {
		t.Fatal("allowed symlink into input")
	}
}

func TestAggregateOutputAndDecodeLimits(t *testing.T) {
	w := &OutputWriter{Root: t.TempDir(), written: maxTotalBytes - 2}
	if _, err := w.Write("too-large", []byte{1, 2, 3}); err == nil {
		t.Fatal("accepted output amplification")
	}
	if _, err := os.Stat(filepath.Join(w.Root, "too-large")); !os.IsNotExist(err) {
		t.Fatal("wrote before budget check", err)
	}
	m := &Manifest{attemptedPixels: maxTotalBytes}
	if _, _, err := m.decode(codeFixture([]int{1}, nil, 1, 1, 11), 1, 1); err == nil {
		t.Fatal("accepted aggregate decompression amplification")
	}
	// Many valid shared ranges can expand far beyond backing-file size.
	n := maxRecords
	dirEnd := 2 + 24*n
	b := make([]byte, dirEnd+65536)
	binary.LittleEndian.PutUint16(b, uint16(n))
	for i := 0; i < n; i++ {
		pos := 2 + 24*i
		copy(b[pos:], "SHARED")
		binary.LittleEndian.PutUint32(b[pos+16:], 65536)
		binary.LittleEndian.PutUint32(b[pos+20:], uint32(dirEnd))
	}
	in := t.TempDir()
	if err := os.WriteFile(filepath.Join(in, "GS2000.DAT"), b, 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "new")
	if _, err := prepareAssets(in, out); err == nil || !strings.Contains(err.Error(), "planned raw extraction") {
		t.Fatal("accepted shared-range amplification", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("created output before aggregate preflight", err)
	}
}
