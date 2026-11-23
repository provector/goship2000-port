package flight

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestLaunchLoadIndependentTerms(t *testing.T) {
	var p Parameters
	binary.LittleEndian.PutUint16(p.Raw[0x42:], 7200)
	s := [4]LoadStation{{true, 0, 1200, 5, 10}, {true, 1, 39, 7, 100}, {true, 0x1001, 3, 200, 500}, {true, 0, 0, 300, 50}}
	// base7200 + (1200/20)*5+10 + (39/20)*7+100 +3*200+500 +0*300+50.
	if got := LaunchLoad(p, s, 0, 0); got != 8767 {
		t.Fatalf("launch term%d", got)
	}
	if got := LaunchLoad(p, s, 6, 8); got != 12767 {
		t.Fatalf("cargo term%d", got)
	}
	if got := LaunchLoad(p, s, 6, 16); got != 10767 {
		t.Fatalf("squad term%d", got)
	}
	if got := LaunchLoad(p, s, 5, 24); got != 8767 {
		t.Fatalf("wrong serialized selector gained cargo%d", got)
	}
	s[1].Present = false
	if got := LaunchLoad(p, s, 0, 0); got != 8660 {
		t.Fatalf("absent station contributes%d", got)
	}
	// Word IMUL/ADD wrap; this is not a real-world mass sanity clamp.
	s = [4]LoadStation{{Present: true, Quantity: -21, UnitTerm: 32767, FixedTerm: 32767}}
	if got := LaunchLoad(p, s, 0, 0); got != 7200 {
		t.Fatalf("signed truncation/wrap%d", got)
	}
}

func TestResidentLaunchLoadOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original source unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Original resident CS0000 maps file600+IP; separate from section0/1.
	for ip, s := range map[int]string{0: "c80a0000568e06603226a1d800bb3a06", 0x66: "8bded1e38b870500b9140099f7f9", 0x8d: "c45efc260107", 0xbf: "24073c06", 0xce: "268107a00f", 0xdf: "268107d007"} {
		want, e := hex.DecodeString(s)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b[0x600+ip:][:len(want)], want) {
			t.Fatalf("resident load source mismatch%04x", ip)
		}
	}
}
