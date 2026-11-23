package cockpit

import (
	"bytes"
	"testing"
)

func testAssets() *Assets {
	a := &Assets{Schema: Schema, Images: map[string]Image{}, Definitions: map[string]Definition{}, Atlases: map[string]Image{}, Sprites: map[string]Image{}, Literals: map[string]string{}, Font: Font{First: 32, Last: 126, Width: 6, Height: 6, Advances: make([]byte, 95), Rows: make([]byte, 95*6)}, Sine: make([]int16, 1025), MapVectors: map[string][]Vector{}}
	for i := range a.Font.Advances {
		a.Font.Advances[i] = 6
	}
	for i := range a.Font.Rows {
		a.Font.Rows[i] = 0xfc
	}
	for _, f := range []string{"APA", "BLK", "COB", "COM", "DEF", "KIA"} {
		p := make([]byte, 64000)
		mask := make([]byte, 64000)
		p[100] = 0
		p[101] = 5
		mask[100] = 1
		a.Images[f+"_CPIT.PIC"] = Image{Width: 320, Height: 200, Pixels: p, FullMask: mask, Split: 32000, Bottom: 99}
		a.Definitions[f] = Definition{Raw: make([]byte, 156)}
	}
	for _, r := range []struct {
		name string
		w, h int
	}{{"heading-strip", 256, 16}, {"vertical-strip", 6, 80}, {"fixed-strip", 5, 32}, {"fixed-scale", 16, 65}, {"digits-opaque", 6, 60}, {"digits-zero-mask", 4, 60}} {
		a.Atlases[r.name] = Image{Width: r.w, Height: r.h, Pixels: make([]byte, r.w*r.h)}
	}
	for _, name := range []string{"hud-arrow-alternate", "hud-arrow", "lateral-marker"} {
		a.Sprites[name] = Image{Width: 15, Height: 7, Pixels: make([]byte, 105)}
	}
	a.Sprites["bearing-marker"] = Image{Width: 7, Height: 3, Pixels: make([]byte, 21)}
	return a
}

func TestOperationSpecificMaskAndImmutableAssets(t *testing.T) {
	a := testAssets()
	frame := bytes.Repeat([]byte{99}, 64000)
	in := Inputs{AircraftID: 0, Arrangement: 0}
	if e := Compose(frame, a, in); e != nil {
		t.Fatal(e)
	}
	if frame[100] != 0 || frame[101] != 99 || frame[63999] != 99 {
		t.Fatal("opaque00/full05/finalpixel behavior", frame[100], frame[101], frame[63999])
	}
	in.Arrangement = 3
	frame = bytes.Repeat([]byte{99}, 64000)
	if e := Compose(frame, a, in); e != nil {
		t.Fatal(e)
	}
	if frame[63999] != 0 || frame[100] != 99 {
		t.Fatal("tail mustcopyentiretail only")
	}
	if a.Images["APA_CPIT.PIC"].Pixels[101] != 5 {
		t.Fatal("sourceatlasmutated")
	}
	in.Arrangement = 2
	frame = bytes.Repeat([]byte{99}, 64000)
	if e := Compose(frame, a, in); e != nil {
		t.Fatal(e)
	}
	if frame[100] != 99 {
		t.Fatal("arrangement2 restoredcockpit")
	}
}

func TestNumericOpaqueVersusZeroSkipAndDomains(t *testing.T) {
	a := testAssets()
	frame := bytes.Repeat([]byte{9}, 64000)
	if e := a.Number(frame, 10, 10, 3, 1); e != nil {
		t.Fatal(e)
	}
	if frame[10*320+10] != 0 {
		t.Fatal("largeglyph00mustbeopaque")
	}
	if e := a.Number(frame, 20, 10, 3, 0x101); e != nil {
		t.Fatal(e)
	}
	if frame[10*320+20] != 9 {
		t.Fatal("smallglyph00mustpreserve")
	}
	for _, c := range []struct {
		value int16
		style uint16
	}{{100, 0x102}, {-32768, 4}, {-10000, 4}} {
		if e := a.Number(frame, 0, 0, c.value, c.style); e == nil {
			t.Fatal("unsafe sourceglyphdomainaccepted", c)
		}
	}
}

func TestTextWrapDoesNotMutateInputAndClip(t *testing.T) {
	a := testAssets()
	frame := make([]byte, 64000)
	s := "AB CD EF"
	if e := a.Text(frame, s, 10, 10, 31, 0x102, Rect{10, 10, 27, 30}); e != nil {
		t.Fatal(e)
	}
	if s != "AB CD EF" {
		t.Fatal("textinputmutated")
	}
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			if frame[y*320+x] != 0 && (x < 10 || x > 27 || y < 10 || y > 30) {
				t.Fatal("textclipping")
			}
		}
	}
}

func TestMapInteriorPixelAndEdgeRejection(t *testing.T) {
	a := testAssets()
	var d Definition
	for off, value := range map[int]int{0x50: 164, 0x52: 134, 0x54: 134, 0x56: 113, 0x58: 195, 0x5a: 155} {
		d.Parameters[off/2] = int16(value)
	}
	in := MapInputs{Grid: make([]byte, 4096), Atlas: make([]byte, 192*144), H0: 299008, H1: 503808}
	for i := range in.Atlas {
		in.Atlas[i] = 7
	}
	frame := bytes.Repeat([]byte{9}, 64000)
	if e := a.DrawMap(frame, d, false, in); e != nil {
		t.Fatal(e)
	}
	count := 0
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			want := byte(9)
			if x >= 134 && x < 195 && y >= 113 && y < 155 {
				want = 7
				count++
			}
			if frame[y*320+x] != want {
				t.Fatal("mapcoverage", x, y)
			}
		}
	}
	if count != 2562 {
		t.Fatal("handpanelarea", count)
	}
	in.H0, in.H1 = 0, 0
	if e := a.DrawMap(frame, d, false, in); e == nil {
		t.Fatal("unresolvededgeRAMsilentlywrapped")
	}
}

func TestRawBarArithmeticAndMatchOperation(t *testing.T) {
	var d Definition
	for off, value := range map[int]int{0x0c: 10, 0x0e: 20, 0x10: 32, 0x12: 20, 0x14: 20, 0x16: 40, 0x18: 30, 0x1a: 20} {
		d.Parameters[off/2] = int16(value)
	}
	frame := bytes.Repeat([]byte{0x4a}, 64000)
	for y := 20; y < 60; y++ {
		for x := 20; x < 24; x++ {
			frame[y*320+x] = 0x9f
		}
	}
	frame[20*320+10] = 0x5a
	// Hand arithmetic: 32-(32*16 SAR5)=16; 40-IDIV(40*3,4)=10;
	// third height16-8=8. Matching is by high nibble, not alpha transparency.
	if e := drawBars(frame, d, BarInputs{LoadOrdinal: 16, FuelNumerator: 3, FuelDivisor: 4, Current: [2]int16{8, 0}}); e != nil {
		t.Fatal(e)
	}
	for _, p := range [][3]int{{11, 35, 0}, {11, 36, 0x4a}, {20, 29, 0}, {20, 30, 0x9f}, {30, 27, 0}, {30, 28, 0x4a}, {10, 20, 0x5a}} {
		if int(frame[p[1]*320+p[0]]) != p[2] {
			t.Fatal("bar source arithmetic/match mismatch", p)
		}
	}
	if e := drawBars(frame, d, BarInputs{}); e == nil {
		t.Fatal("zero source divisor silently invented a result")
	}
}

func TestAttitudeUsesCachedTrigonometricTermsAndExactIndex(t *testing.T) {
	a := testAssets()
	d := Definition{SpanReplacement: 0x1965}
	d.Parameters[0x4c/2] = 160
	d.Parameters[0x4e/2] = 60
	frame := bytes.Repeat([]byte{0x19}, 64000)
	frame[61*320+160] = 0x18
	// 0d93 zero-angle caches {sin0,cos0,sin0}={0,16384,0}.
	// IDIV by24064 then SAR2 gives43/-44; the lower half is replaced only
	// where the destination exactly equals DH=19h, with DL=65h.
	a.attitude(frame, d, [3]int16{0, 16384, 0})
	if frame[59*320+160] != 0x19 || frame[62*320+160] != 0x65 || frame[61*320+160] != 0x18 || frame[73*320+160] != 0x19 {
		t.Fatal("attitude cached trig/operation-specific exact replacement")
	}
}
