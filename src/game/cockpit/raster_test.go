package cockpit

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestStrictBorrowLineHandCases(t *testing.T) {
	// Expectations enumerate the original SUB/JAE loop by hand. In particular,
	// exact zero after subtraction does not take the minor-axis step.
	cases := []struct {
		name   string
		end    [2]int
		points [][2]int
	}{
		{"point", [2]int{0, 0}, [][2]int{{0, 0}}},
		{"horizontal", [2]int{4, 0}, [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}},
		{"vertical", [2]int{0, 4}, [][2]int{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}}},
		{"diagonal", [2]int{4, 4}, [][2]int{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 4}}},
		{"even shallow tie", [2]int{4, 2}, [][2]int{{0, 0}, {1, 0}, {2, 1}, {3, 1}, {4, 2}}},
		{"odd shallow", [2]int{5, 2}, [][2]int{{0, 0}, {1, 0}, {2, 1}, {3, 1}, {4, 2}, {5, 2}}},
		{"shallow third", [2]int{6, 2}, [][2]int{{0, 0}, {1, 0}, {2, 1}, {3, 1}, {4, 1}, {5, 2}, {6, 2}}},
		{"even steep tie", [2]int{2, 4}, [][2]int{{0, 0}, {0, 1}, {1, 2}, {1, 3}, {2, 4}}},
		{"negative shallow", [2]int{4, -2}, [][2]int{{0, 0}, {1, 0}, {2, -1}, {3, -1}, {4, -2}}},
		{"negative steep", [2]int{2, -4}, [][2]int{{0, 0}, {0, -1}, {1, -2}, {1, -3}, {2, -4}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				p := make([]byte, 64000)
				x0, y0, x1, y1 := 100, 100, 100+tc.end[0], 100+tc.end[1]
				if reverse {
					x0, x1 = x1, x0
					y0, y1 = y1, y0
				}
				Line(p, x0, y0, x1, y1, 14, Full)
				want := make([]byte, 64000)
				for _, v := range tc.points {
					want[(100+v[1])*320+100+v[0]] = 14
				}
				if !bytes.Equal(p, want) {
					t.Fatalf("reverse=%v emitted unexpected pixels", reverse)
				}
			}
		})
	}
	// Preserve emitted-pixel clipping rather than inventing endpoint clipping.
	p := make([]byte, 64000)
	Line(p, -1, 0, 3, 2, 14, Rect{0, 0, 2, 1})
	want := make([]byte, 64000)
	want[0] = 14
	want[321] = 14
	want[322] = 14
	if !bytes.Equal(p, want) {
		t.Fatal("line clip changed the independently stepped interior")
	}
}

func rasterFixture(t *testing.T, name, hash string) []byte {
	t.Helper()
	b, e := os.ReadFile(testfixture.Path("cockpit-raster/") + name)
	if e != nil {
		t.Fatal(e)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != hash {
		t.Fatalf("original fixture %s hash %s", name, got)
	}
	return b
}

func baseAssets(t *testing.T) *Assets {
	t.Helper()
	raw := rasterFixture(t, "base-vector.bin", "6d613056e47976f4f81a09753705aff50bc0d728305756c13ba787c646e7deab")
	want := []byte{1, 4, 253, 253, 253, 3, 3, 3, 3, 253, 253, 253, 15, 5, 255, 254, 255, 2, 1, 1, 255, 0, 1, 255, 255, 254, 0}
	if !bytes.Equal(raw, want) {
		t.Fatal("original B vector operands changed")
	}
	a := testAssets()
	a.Sine[1024] = 16384
	a.MapVectors["map-07df"] = []Vector{
		{Colour: 1, Points: [][2]int16{{-3, -3}, {-3, 3}, {3, 3}, {3, -3}, {-3, -3}}},
		{Colour: 15, Points: [][2]int16{{-1, -2}, {-1, 2}, {1, 1}, {-1, 0}, {1, -1}, {-1, -2}}},
	}
	return a
}

func TestBaseExactOriginalRectangleAndScaledOwnership(t *testing.T) {
	before := rasterFixture(t, "base-before.gray8", "507ae70b74254db106e8824c1b8f76c47ad42727d39bf14facaca7b5ddb45d53")
	after := rasterFixture(t, "base-after.gray8", "3984807f778eeb8cb57e24fdab40c28feb82411f53cc6055d65c949b52bea0d6")
	a := baseAssets(t)
	draw := func(p []byte) error {
		return a.DrawMissionMarkers(p, &MapMission{Base: MapObjective{Status: 3, H0: 157, H1: 43}}, func(x, y uint32) (int, int) { return int(x), int(y) }, -3279, 8, Rect{2, 3, 226, 164})
	}
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprintf("scale%d", scale), func(t *testing.T) {
			// Independent subpixel backgrounds verify that enlargement does not collapse
			// destination ownership into a single low-resolution scratch framebuffer.
			p := make([]byte, 64000*scale*scale)
			want := make([]byte, len(p))
			stride := 320 * scale
			for y := 0; y < 200*scale; y++ {
				for x := 0; x < 320*scale; x++ {
					v := byte(80 + (y%scale)*scale + x%scale)
					at := y*stride + x
					p[at] = v
					want[at] = v
				}
			}
			for y := 0; y < 14; y++ {
				for x := 0; x < 14; x++ {
					for sy := 0; sy < scale; sy++ {
						for sx := 0; sx < scale; sx++ {
							at := ((37+y)*scale+sy)*stride + (151+x)*scale + sx
							p[at] = before[y*14+x]
							want[at] = after[y*14+x]
						}
					}
				}
			}
			if e := LogicalPass(p, scale, draw); e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(p, want) {
				for i, v := range p {
					if v != want[i] {
						t.Fatalf("original rectangle/outside mismatch(%d,%d): %d want %d", i%stride, i/stride, v, want[i])
					}
				}
			}
		})
	}
}

func TestBaseMFDPostpassFromOriginalBrightness(t *testing.T) {
	after := rasterFixture(t, "base-after.gray8", "3984807f778eeb8cb57e24fdab40c28feb82411f53cc6055d65c949b52bea0d6")
	table := rasterFixture(t, "brightness.bin", "472351265b3c7b0e092e4b85b590b9605ce25c1e31277d1e7e65b6300d7accd8")
	// Independent original-table operands: black/blue become21; white becomes31.
	if table[0]+21 != 21 || table[1]+21 != 21 || table[15]+21 != 31 {
		t.Fatal("original monochrome bank operands")
	}
	a := testAssets()
	a.ScenePresentation.ConfirmationRemap = table
	d := Definition{}
	d.Parameters[0x54/2] = 151
	d.Parameters[0x56/2] = 37
	d.Parameters[0x58/2] = 164
	d.Parameters[0x5a/2] = 50
	for _, colour := range []bool{false, true} {
		p := bytes.Repeat([]byte{88}, 64000)
		for y := 0; y < 14; y++ {
			copy(p[(37+y)*320+151:], after[y*14:(y+1)*14])
		}
		flags := byte(0)
		if colour {
			flags = 1
		}
		if e := a.remapMFD(p, d, false, flags); e != nil {
			t.Fatal(e)
		}
		for y := 0; y < 14; y++ {
			for x := 0; x < 14; x++ {
				v := after[y*14+x]
				if !colour {
					v = table[v] + 21
				}
				if p[(37+y)*320+151+x] != v {
					t.Fatal("MFD contextual postpass", colour, x, y)
				}
			}
		}
		if p[37*320+150] != 88 || p[37*320+165] != 88 {
			t.Fatal("postpass escaped bounds")
		}
	}
}

func TestOtherMissionVectorsHandArtwork(t *testing.T) {
	// Literal original015b vectors P075a/S0722/F07c4. Axis-aligned controls
	// independently retain the source artwork; the B fix does not redraw them.
	for _, tc := range []struct {
		name    string
		vectors []Vector
		rows    []string
	}{
		{"P", []Vector{{Colour: 15, Points: [][2]int16{{-1, -2}, {-1, 2}, {1, 2}, {1, 0}, {-1, 0}}}}, []string{"###", "#.#", "###", "#..", "#.."}},
		{"S", []Vector{{Colour: 15, Points: [][2]int16{{-1, -2}, {1, -2}, {1, 0}, {-1, 0}, {-1, 2}, {1, 2}}}}, []string{"###", "#..", "###", "..#", "###"}},
		{"F", []Vector{{Colour: 15, Points: [][2]int16{{-1, -2}, {-1, 2}, {1, 2}}}, {Colour: 15, Points: [][2]int16{{-1, 0}, {1, 0}}}}, []string{"###", "#..", "###", "#..", "#.."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testAssets()
			a.Sine[1024] = 16384
			p := make([]byte, 64000)
			vectors := []Vector{{Colour: 1, Points: [][2]int16{{-3, -3}, {-3, 3}, {3, 3}, {3, -3}, {-3, -3}}}}
			vectors = append(vectors, tc.vectors...)
			a.DrawVectors(p, vectors, 100, 100, 0, 8, 0, Full)
			want := make([]byte, 64000)
			for y := 97; y <= 103; y++ {
				for x := 97; x <= 103; x++ {
					if x == 97 || x == 103 || y == 97 || y == 103 {
						want[y*320+x] = 1
					}
				}
			}
			for y, row := range tc.rows {
				for x, ch := range row {
					if ch == '#' {
						want[(98+y)*320+99+x] = 15
					}
				}
			}
			if !bytes.Equal(p, want) {
				t.Fatal("source marker artwork changed")
			}
		})
	}
}

func TestOriginalLineOperandWitness(t *testing.T) {
	b := rasterFixture(t, "vga-line.bin", "79e815aa9974cf90022956b32fa67c3f88b49f77268c34cc4a72f29e467afb86")
	// Original selected-VGA section7 file3e550+IP. All four octant loops
	// initialize floor(major/2), include the end, and skip on NO borrow.
	for _, ip := range []int{0x76b, 0x781, 0x796, 0x7ab} {
		at := ip - 0x729
		if !bytes.Equal(b[at:at+3], []byte{0xd1, 0xeb, 0x41}) {
			t.Fatalf("SHR BX,1 / INC CX at%x", ip)
		}
	}
	for _, ip := range []int{0x773, 0x785, 0x79a, 0x7b3} {
		at := ip - 0x729
		if !bytes.Equal(b[at:at+3], []byte{0x2b, 0xda, 0x73}) {
			t.Fatalf("SUB BX,DX / JAE at%x", ip)
		}
	}
}

func TestCockpitRouteUsesOriginalTie(t *testing.T) {
	a := testAssets()
	p := make([]byte, 64000)
	in := MapInputs{H0: 10000, H1: 10000, Observer: 1}
	in.Routes[0] = MapRoute{Index: 1, Mode: 1, H0: 10000, H1: 10000, Points: [3]MapPoint{{H0: 10000, H1: 10000}, {H0: 10682, H1: 9545}}}
	// Source divisors341,-455 produce(100,100)->(102,101). The middle
	// pixel is(101,100), independently of waypoint-vector artwork.
	a.drawMapRoutes(p, in, 100, 100, Full)
	want := make([]byte, 64000)
	want[100*320+100] = 7
	want[100*320+101] = 7
	want[101*320+102] = 7
	if !bytes.Equal(p, want) {
		t.Fatal("route segment lost selected-VGA strict-borrow tie")
	}
}
