package render3d

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestOriginalProjectionExceptionOperands(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), "GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original instruction comparison")
	}
	if err != nil {
		t.Fatal(err)
	}
	// The installed resident handler has exact F7/FF dispatch, signed
	// reduced division and the signed -64/+64 bounds used by the hand test.
	for _, check := range []struct {
		ip   int
		want []byte
	}{
		{0x1f97, []byte{0x80, 0x3f, 0xf7}},
		{0x1fa3, []byte{0x80, 0x3f, 0xff}},
		{0x1fd2, []byte{0x8a, 0xc4, 0x8a, 0xe2, 0x8a, 0xd6, 0x8b, 0xdf, 0xf7, 0xfb, 0xbb, 0x00, 0x83, 0x83, 0xf8, 0xc0}},
		{0x1fe6, []byte{0x83, 0xf8, 0x40}},
		{0x201a, []byte{0xb8, 0x00, 0x80, 0x79, 0x01, 0x48, 0xcf}},
		{0x20c5, []byte{0x8b, 0x47, 0x04, 0x3b, 0x47, 0x0c, 0x75, 0x1f}},
	} {
		at := 0x3df0 + check.ip
		if !bytes.Equal(raw[at:at+len(check.want)], check.want) {
			t.Fatalf("unsupported resident source at037f:%04x", check.ip)
		}
	}
	// Section0 comes from its own loader descriptor, not section1's base.
	desc := 0x600 + 0x851*16 + 0xb71
	word := func(i int) int { return int(binary.LittleEndian.Uint16(raw[desc+2*i:])) }
	body := 16 * (word(2) + (word(5)+3)/4)
	base := body + 16*(0x975-word(0))
	if base != 0xa120 || !bytes.Equal(raw[base+0xf4:base+0xfe], []byte{0x9a, 0xf5, 0x1f, 0x7f, 0x03, 0x9a, 0x21, 0x20, 0x7f, 0x03}) {
		t.Fatal("section0 handler installation calls do not match loader mapping")
	}
}

func originalBundle(t testing.TB, name string) *world.Bundle {
	t.Helper()
	base := filepath.Join(testfixture.Path("installation"))
	read := func(n string) []byte {
		b, e := os.ReadFile(filepath.Join(base, n))
		if os.IsNotExist(e) {
			t.Skip("optional original-source renderer comparison")
		}
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	models, e := world.ParseModels(read("OBJFILE"), read("OBJTYPES.DAT"))
	if e != nil {
		t.Fatal(e)
	}
	w, geometries, e := world.ParseWorld(name, read(name), read(name+"BMP.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	b := &world.Bundle{Models: make([]string, len(models)), Geometries: map[string]*world.Geometry{}, Worlds: map[string]*world.World{name: w}, Palettes: map[string]world.Palette{}}
	for i, g := range models {
		b.Models[i] = g.ID
		b.Geometries[g.ID] = g
	}
	for _, g := range geometries {
		b.Geometries[g.ID] = g
	}
	// Exercise the same standard geometry payload used by prepared playback.
	// This transport check is separate from hand-derived raster expectations.
	for _, g := range b.Geometries {
		encoded, e := world.EncodeGLB(g)
		if e != nil {
			t.Fatal(e)
		}
		g.Vertices, e = world.DecodeGLB(encoded)
		if e != nil {
			t.Fatal(e)
		}
	}
	gs := read("GS2.GS2")
	raw := gs[0x4bc50 : 0x4bc50+2050]
	b.Sine = make([]int16, 1025)
	for i := range b.Sine {
		b.Sine[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	for _, bank := range []string{"DAY", "GREY", "DARK"} {
		id := name[:4] + bank + ".PAL"
		b.Palettes[id] = world.Palette{ID: id, RGB6: read(id)}
	}
	return b
}

func sourceTrainingScene(t testing.TB) Scene {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(testfixture.Path("working"), "step28", "run01", "GS2000CD", "OBJECT.DAT"))
	if os.IsNotExist(e) {
		t.Skip("optional original-written Training handoff")
	}
	if e != nil {
		t.Fatal(e)
	}
	s := Scene{World: "GULF5", Camera: Camera{Pose: Pose{H0: 350760, H1: 345568, Height: 512}, Angles: [3]uint16{0, 0xe000, 0}}, View: FullView(), GraphicsOptions: 0xdf, PaletteBank: "DARK"}
	for _, i := range []int{35, 36, 37, 38, 39, 40, 41} {
		row := raw[i*32 : (i+1)*32]
		s.Instances = append(s.Instances, Instance{ID: string(rune('A' + i - 35)), ModelID: int(row[12]), Pose: Pose{H0: binary.LittleEndian.Uint32(row[2:]), H1: binary.LittleEndian.Uint32(row[6:])}, Oriented: binary.LittleEndian.Uint16(row)&8 != 0, Callback: byte(binary.LittleEndian.Uint16(row) >> 8)})
	}
	return s
}

func TestOriginalSourceStationaryScene(t *testing.T) {
	r, e := New(originalBundle(t, "GULF5"))
	if e != nil {
		t.Fatal(e)
	}
	scene := sourceTrainingScene(t)
	state := &VisualState{}
	dst := make([]byte, 64000)
	for i := range dst {
		dst[i] = 0xa6
	}
	stats, e := r.Draw(scene, state, dst)
	if e != nil {
		t.Fatal(e)
	}
	if stats.Cells == 0 || stats.Primitives == 0 || stats.Instances == 0 {
		t.Fatalf("empty genuine scene%+v", stats)
	}
	h := sha256.Sum256(dst)
	t.Logf("source-linked synthetic scene stats=%+v indexedSHA=%s; not an original-frame comparison", stats, hex.EncodeToString(h[:]))
	if _, e = r.Palette(scene, state); e != nil {
		t.Fatal(e)
	}
	if state.Phase != 0 {
		t.Fatal("render advanced preview phase")
	}
}

func BenchmarkSourcePopulatedScene(b *testing.B) {
	r, e := New(originalBundle(b, "GULF5"))
	if e != nil {
		b.Fatal(e)
	}
	scene := sourceTrainingScene(b)
	dst := make([]byte, 64000)
	state := &VisualState{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e = r.Draw(scene, state, dst); e != nil {
			b.Fatal(e)
		}
	}
}
