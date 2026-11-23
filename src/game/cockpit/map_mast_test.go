package cockpit

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func mapMastOriginal(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source bytes unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMapMastCallersAndActualDriverOperands(t *testing.T) {
	b := mapMastOriginal(t)
	for _, q := range []struct {
		base, ip int
		hex      string
	}{
		{0x1dc60, 0x1244, "6aff6a086839fe685501"}, // selected=-1, shift8, -455,341
		{0x1dc60, 0x1780, "f606e100017531803ebe3b00"},
		{0x1dc60, 0x1796, "6a158e06be3326ff36061c26ff36041c26ff36021c26ff36001c9aa1197f03"},
		{0x1dc60, 0x1cd7, "26a1fe1b2639060a1c"}, // secondary anchor-height equality
		{0x1dc60, 0x222a, "eb16"},               // target path reaches same bezel as empty
		{0x1dc60, 0x223a, "9a97197f0383c40a"},
		{0x1dc60, 0x2256, "6a046a7f6a37"},                // flags4, y127, x55
		{0x1dc60, 0x2156, "68c8006840016a786a00"},        // mast text area0,120..320,200
		{0x1dc60, 0x217d, "68b4006a00"},                  // namey180 centered
		{0x1dc60, 0x21ec, "68b4006a64"},                  // range100,180
		{0x1dc60, 0x2212, "68b40068c800"},                // bearing200,180
		{0x1bb0, 0xd73, "6a000e689d078b461080c402"},      // map sweep0208 shadow
		{0x3df0, 0x34d0, "ff07ff4702ff4704ff470633d252"}, // four endpoints +1, black0
		{0x3e550, 0xf, "eac80f471d"},                     // selected1929 sprite target
		{0x3e550, 0xfe7, "f7460e0100"},                   // source sprite flags are BP+0e
		{0x3e550, 0x1005, "f7460e0400"},                  // flags4 skips local clip
		{0x3e550, 0x10ae, "ac0ac0740326880547e2f5"},      // LODSB; OR; JZ; write; INC; LOOP
		{0x3e550, 0x87, "ea340e471d"},                    // selected19a1 remap target
		{0x3e550, 0xe55, "8a660e8b560a2b5606428b4e0c2b4e0841bb340d"},
		{0x3e550, 0xe6c, "268a052ed702c4aa"}, // destination read, CS XLAT, add bank, store
	} {
		want, err := hex.DecodeString(q.hex)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b[q.base+q.ip:][:len(want)], want) {
			t.Fatalf("source operand file %06x (%04x) differs", q.base+q.ip, q.ip)
		}
	}
}

func TestMapRouteHandCoordinatesAndSourceGates(t *testing.T) {
	a := testAssets()
	in := MapInputs{H0: 300000, H1: 300000}
	in.Routes[0] = MapRoute{Index: 1, Mode: 3, H0: 300000, H1: 300000,
		Points: [3]MapPoint{{301705, 298635}, {303410, 298635}, {305115, 300000}}}
	before := in.Routes
	p := bytes.Repeat([]byte{99}, 64000)
	a.drawMapRoutes(p, in, 160, 120, Full)
	// DIV341/-455 yields (5,3),(10,3),(15,0). Source route0 colour1;
	// active1 adds ownship-to-point1, in addition to point1->2 and2->3.
	for _, xy := range [][2]int{{160, 120}, {165, 123}, {167, 123}, {170, 123}, {175, 120}} {
		if p[xy[1]*320+xy[0]] != 1 {
			t.Fatalf("hand projected route pixel %+v", xy)
		}
	}
	if !reflect.DeepEqual(before, in.Routes) {
		t.Fatal("drawing mutated authoritative route snapshot")
	}
	in.Routes[0].Index = 2
	p = bytes.Repeat([]byte{99}, 64000)
	a.drawMapRoutes(p, in, 160, 120, Rect{166, 122, 174, 124})
	if p[120*320+160] != 99 || p[123*320+167] != 1 || p[120*320+175] != 99 {
		t.Fatal("active point or clipping gate changed source route segments")
	}
	for _, q := range []struct {
		index int8
		mode  byte
	}{{0, 3}, {1, 0}} {
		in.Routes[0].Index, in.Routes[0].Mode = q.index, q.mode
		p = bytes.Repeat([]byte{99}, 64000)
		a.drawMapRoutes(p, in, 160, 120, Full)
		if !bytes.Equal(p, bytes.Repeat([]byte{99}, 64000)) {
			t.Fatal("zero source index/mode must suppress row")
		}
	}
}

func TestAllAircraftPanelColourGatesAndSourceBrightness(t *testing.T) {
	original := mapMastOriginal(t)
	a := testAssets()
	a.ScenePresentation.ConfirmationRemap = append([]byte(nil), original[0x3e550+0xd34:0x3e550+0xe34]...)
	for id := 0; id < 8; id++ {
		family, _ := Family(id)
		raw, err := os.ReadFile(testfixture.Path("installation/") + family + "_CPIT.DAT")
		if err != nil {
			t.Fatal(err)
		}
		d := Definition{}
		for n := range d.Parameters {
			d.Parameters[n] = int16(binary.LittleEndian.Uint16(raw[2*n:]))
		}
		flags := byte(0)
		if id == 6 || id == 7 { // GS writer0/1/2/3/5/6/4/4, SHIP selector4 setsE1.0.
			flags = 1
		}
		for _, secondary := range []bool{false, true} {
			t.Run(fmt.Sprintf("aircraft%d-secondary%t", id, secondary), func(t *testing.T) {
				p := bytes.Repeat([]byte{10}, 64000)
				if err := a.remapMFD(p, d, secondary, flags); err != nil {
					t.Fatal(err)
				}
				base := 0x54
				if secondary {
					base = 0x60
				}
				left, top, right, bottom := int(d.Parameters[base/2]), int(d.Parameters[(base+2)/2]), int(d.Parameters[(base+4)/2]), int(d.Parameters[(base+6)/2])
				admit := flags == 0 && (!secondary || d.Parameters[0x52/2] == d.Parameters[0x5e/2])
				for y := 0; y < 200; y++ {
					for x := 0; x < 320; x++ {
						want := byte(10)
						if admit && x >= left && x <= right && y >= top && y <= bottom {
							want = 29 // source table[10]=8; +15h=29, independently literal checked.
						}
						if p[y*320+x] != want {
							t.Fatalf("source remap extent/polarity at%d,%d", x, y)
						}
					}
				}
			})
		}
	}
}

func TestActualMastArtOverlayKeepsPreviewAndBothScales(t *testing.T) {
	b := mapMastOriginal(t)
	const at = 0x3e550 + 0x61d6
	if binary.LittleEndian.Uint16(b[at:]) != 211 || binary.LittleEndian.Uint16(b[at+2:]) != 73 {
		t.Fatal("source mast dimensions")
	}
	art := b[at+4 : at+4+211*73]
	a := testAssets()
	a.Sprites["mast-view"] = Image{Width: 211, Height: 73, Pixels: append([]byte(nil), art...)}
	for _, scale := range []int{1, 2} {
		dst := bytes.Repeat([]byte{0x61}, 64000*scale*scale)
		if err := a.DrawMastFrameScaled(dst, scale); err != nil {
			t.Fatal(err)
		}
		inside := 0
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				want := byte(0x61)
				if x >= 55 && x < 266 && y >= 127 && y < 200 {
					v := art[(y-127)*211+x-55]
					if v != 0 {
						want = v
						if x >= 69 && x <= 251 && y >= 132 && y <= 196 {
							inside++
						}
					}
				}
				for sy := 0; sy < scale; sy++ {
					for sx := 0; sx < scale; sx++ {
						if dst[(y*scale+sy)*(320*scale)+x*scale+sx] != want {
							t.Fatalf("source zero mask/scale%d at%d,%d", scale, x, y)
						}
					}
				}
			}
		}
		if inside != 786 {
			t.Fatal("actual art bezel pixels within preview rectangle", inside)
		}
	}
}

func TestCockpitMapSweepHasSourceOffsetBlackShadow(t *testing.T) {
	a := testAssets()
	a.Sine[1024] = 16384
	a.MapVectors["map-079d"] = []Vector{{Colour: 15, Points: [][2]int16{{0, 0}, {1, 0}}}}
	var d Definition
	for off, v := range map[int]int16{0x50: 100, 0x52: 100, 0x54: 80, 0x56: 80, 0x58: 120, 0x5a: 120} {
		d.Parameters[off/2] = v
	}
	in := MapInputs{Grid: make([]byte, 4096), Atlas: bytes.Repeat([]byte{99}, 192*144), H0: 300000, H1: 300000, DrawOwnship: true}
	p := make([]byte, 64000)
	if err := a.DrawMap(p, d, false, in); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		x, y int
		want byte
	}{{100, 100, 15}, {101, 100, 15}, {101, 101, 0}, {102, 101, 0}, {100, 101, 99}} {
		if p[q.y*320+q.x] != q.want {
			t.Fatal("source0208 shadow/vector layering", q, p[q.y*320+q.x])
		}
	}
}
