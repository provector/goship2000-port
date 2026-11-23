package render3d

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fillWitness struct {
	Material      uint16
	Writes        int
	Spans         []struct{ Y, Left, Right int }
	ReadAddresses []int `json:"read_addresses"`
	Rectangle     struct{ X, Y, Width, Height int }
	CSStart       int `json:"cs_start"`
	CSLength      int `json:"cs_length"`
}

// Expected pixels and read offsets come from original debugger memory replies,
// not from another invocation of this renderer or a generated native golden.
func TestOriginalJungleOperations(t *testing.T) {
	for _, id := range []string{"01", "03"} {
		t.Run(id, func(t *testing.T) {
			read := func(name string) []byte {
				b, e := os.ReadFile(filepath.Join(testfixture.Path("rendering"), "stepM2", name))
				if e != nil {
					t.Fatal(e)
				}
				return b
			}
			var w fillWitness
			if e := json.Unmarshal(read(id+"-spans.json"), &w); e != nil {
				t.Fatal(e)
			}
			carrier := &Carrier{DS: read(id + "-ds.bin")}
			before, want := make([]byte, 64000), make([]byte, 64000)
			if id == "01" {
				carrier.CS = read("01-cs.bin")
				before = read("01-before.bin")
				want = read("01-after.bin")
			} else {
				carrier.CS = bytes.Repeat([]byte{0xcc}, 65536) // poison uncaptured bytes
				copy(carrier.CS[w.CSStart:], read("03-cs-consumed.bin"))
				carrier.Readable = []CarrierRange{{w.CSStart, w.CSStart + w.CSLength}}
				before = bytes.Repeat([]byte{0xa5}, 64000)
				copy(want, before)
				a, b := read("03-before.gray8"), read("03-after.gray8")
				rect := w.Rectangle
				for y := 0; y < rect.Height; y++ {
					at := (rect.Y+y)*320 + rect.X
					copy(before[at:at+rect.Width], a[y*rect.Width:(y+1)*rect.Width])
					copy(want[at:at+rect.Width], b[y*rect.Width:(y+1)*rect.Width])
				}
			}
			var addresses []int
			writes := 0
			for _, s := range w.Spans {
				writes += s.Right - s.Left + 1
				for x := s.Left + ((s.Right - s.Left + 1) & 1); x <= s.Right; x += 2 {
					addresses = append(addresses, s.Y*320+x)
				}
			}
			if !reflect.DeepEqual(addresses, w.ReadAddresses) || writes != w.Writes {
				t.Fatal("span/read ledger differs from original capture")
			}
			cs, ds := bytes.Clone(carrier.CS), bytes.Clone(carrier.DS)
			for _, scale := range []int{1, 2} {
				dst := expand(before, scale)
				for _, s := range w.Spans {
					for dy := 0; dy < scale; dy++ {
						if fillSpan(dst, s.Y*scale+dy, s.Left*scale, (s.Right+1)*scale-1, w.Material, 1, carrier) {
							t.Fatal("captured read rejected")
						}
					}
				}
				expected := expand(want, scale)
				if !bytes.Equal(dst, expected) {
					n := 0
					first := -1
					for i, v := range dst {
						if v != expected[i] {
							n++
							if first < 0 {
								first = i
							}
						}
					}
					t.Fatalf("scale%d: %d mismatches, first%d", scale, n, first)
				}
				if !bytes.Equal(cs, carrier.CS) || !bytes.Equal(ds, carrier.DS) {
					t.Fatal("drawing mutated captured source memory")
				}
				if out := os.Getenv("GS2000_M2_MATERIAL_OUTPUT"); out != "" {
					path := filepath.Join(out, fmt.Sprintf("native%s-scale%d.gray8", id, scale))
					file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
					if err != nil {
						t.Fatal(err)
					}
					_, writeErr := file.Write(dst)
					closeErr := file.Close()
					if writeErr != nil || closeErr != nil {
						t.Fatalf("export: %v %v", writeErr, closeErr)
					}
				}
				t.Logf("scale%d: %d logical writes/%d original pair reads; exact%s", scale, writes, len(addresses), map[bool]string{true: "64000-byte buffer", false: "1248-pixel rectangle + no outside writes"}[id == "01"])
			}
		})
	}
}
func expand(src []byte, scale int) []byte {
	dst := make([]byte, len(src)*scale*scale)
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			for j := 0; j < scale; j++ {
				for i := 0; i < scale; i++ {
					dst[(y*scale+j)*320*scale+x*scale+i] = src[y*320+x]
				}
			}
		}
	}
	return dst
}

func TestCarrierReadWidthAndUnsupportedDomains(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dst := bytes.Repeat([]byte{0xaa}, 64000*scale*scale)
			// Source JZ after the plain odd leading STOSB: no carrier is required.
			if fillSpan(dst, 0, 3*scale, 4*scale-1, 0x74, 1, nil) {
				t.Fatal("one logical pixel incorrectly depends on CS/DS")
			}
			for x := 3 * scale; x < 4*scale; x++ {
				if dst[x] != 0x74 {
					t.Fatal("plain DL leading byte changed")
				}
			}
			c := &Carrier{CS: make([]byte, 4), DS: make([]byte, 4), Readable: []CarrierRange{{3, 4}}}
			c.CS[3] = 1
			c.DS[3] = 2
			// Only byte3 is read: the paired destination4 needs no carrier byte4.
			if fillSpan(dst, 0, 3*scale, 5*scale-1, 0x74, 1, c) {
				t.Fatal("byte read incorrectly widened")
			}
			for x := 3 * scale; x < 5*scale; x++ {
				if dst[x] != 0x74 {
					t.Fatal("hand q0 pair")
				}
			}
			c.Readable = []CarrierRange{} // explicitly unknown, not an all-zero witness
			if !fillSpan(dst, 0, 3*scale, 5*scale-1, 0x74, 1, c) {
				t.Fatal("unknown carrier region silently admitted")
			}
			c.Readable = []CarrierRange{{0, 3}}
			if !fillSpan(dst, 0, 3*scale, 5*scale-1, 0x74, 1, c) {
				t.Fatal("half-open range boundary silently admitted")
			}
		})
	}
}
