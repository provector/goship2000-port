package render3d

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSourceMemoryMaterialGenericDispatchOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Section7 original1d47, file3e550+IP. These instructions inspect only
	// the material high byte and setup bit: there is no Wreck/model-ID test.
	for ip, text := range map[int]string{
		0x3f8: "0af6750f8af280f607a03e59a8017403e9cd00",
		0x515: "8bc241d1e97303aa740f", // Odd STOSB then JZ: one pixel reads no carrier.
		0x51f: "2e3225322580e40332c432e0abe2f181c3400183c604",
	} {
		want, err := hex.DecodeString(text)
		if err != nil {
			t.Fatalf("invalid source-check bytes at1d47:%04x: %v", ip, err)
		}
		if !bytes.Equal(raw[0x3e550+ip:][:len(want)], want) {
			t.Fatalf("unsupported original material operand1d47:%04x", ip)
		}
	}
}

func TestSourceMemoryMaterialSuppliedCarrierHandTrace(t *testing.T) {
	carrier := &Carrier{CS: make([]byte, 64000), DS: make([]byte, 64000)}
	carrier.CS[0], carrier.DS[0] = 1, 2
	carrier.CS[2], carrier.DS[2] = 0, 1
	carrier.CS[4], carrier.DS[4] = 3, 0
	beforeCS, beforeDS := append([]byte(nil), carrier.CS...), append([]byte(nil), carrier.DS...)
	dst := bytes.Repeat([]byte{0xaa}, 64000)
	if fillSpan(dst, 0, 0, 5, 0x24, 1, carrier) {
		t.Fatal("actual carrier must use the source operation")
	}
	// Hand source AX trace starts2324. q=0 ->2424, q=1 ->2425,
	// q=3 ->2526. Each little-endian word emits low byte first.
	if !bytes.Equal(dst[:6], []byte{0x24, 0x24, 0x25, 0x24, 0x26, 0x25}) || dst[6] != 0xaa {
		t.Fatalf("source24 feedback/extent: %x", dst[:7])
	}
	if !bytes.Equal(carrier.CS, beforeCS) || !bytes.Equal(carrier.DS, beforeDS) {
		t.Fatal("fill mutated supplied source carriers")
	}
	clear(carrier.CS)
	clear(carrier.DS)
	// Odd length emits plain DL first; special fill does not swap the seed
	// pair on odd Y as the ordinary checker branch does.
	if fillSpan(dst, 1, 3, 9, 0x24, 1, carrier) || !bytes.Equal(dst[323:330], []byte{0x24, 0x27, 0x24, 0x27, 0x27, 0x24, 0x27}) {
		t.Fatalf("odd source span: %x", dst[323:330])
	}
}

func TestSourceMemoryMaterialFallbackStaysInSourceIndexFamily(t *testing.T) {
	dst, repeated := make([]byte, 64000), make([]byte, 64000)
	for raw := 1; raw < 256; raw++ {
		lo := byte(raw)
		for y := 0; y < 8; y++ {
			if !fillSpan(dst, y, 0, 23, uint16(lo), 1, nil) || !fillSpan(repeated, y, 0, 23, uint16(lo), 1, nil) {
				t.Fatal("missing carrier must remain explicitly diagnostic")
			}
			for x := 0; x < 24; x++ {
				if dst[y*320+x]&0xfc != lo&0xfc {
					t.Fatalf("diagnostic%d emitted a foreign source-index family%02x", raw, dst[y*320+x])
				}
			}
		}
		if !bytes.Equal(dst, repeated) {
			t.Fatal("identical diagnostic fill changed with drawing")
		}
	}
	// The source bit-clear branch is a distinct24/23 checker, not the
	// configured memory branch's24..27 family. Do not normalize it.
	if fillSpan(dst, 0, 0, 3, 0x24, 0, nil) || !bytes.Equal(dst[:4], []byte{0x24, 0x23, 0x24, 0x23}) {
		t.Fatal("ordinary source implicit pair changed")
	}
}
