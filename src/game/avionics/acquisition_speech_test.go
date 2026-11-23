package avionics

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestAcquisitionSpeechOriginalOperands(t *testing.T) {
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for ip, raw := range map[int]string{
		0x1d23: "26833ed10a00740b26833ed10a077403e9a207", // camera0/7, arrangement-independent
		0x2276: "26833eb90a007503e91102",                 // AB9 nonzero
		0x2285: "26833e0b0b007503e90202",                 // B0B nonzero
		0x2294: "26a1b30a8e06e033263906b90a7459",         // AB3 != AB9
		0x22a3: "26a1b90a8e06f63326a3b30af606de00807446", // cache before speech gate
		0x22bd: "2680bf6a250176052bc0eb0490b81800",       // unsigned kind<=1 air phrase base18
		0x22d4: "26833efaffe07d06052d00eb1290",           // signed left boundary
		0x22e2: "26833efaff207e06052e00eb0490053000",     // signed right / inclusive center
		0x22f3: "509a0b00440883c402",                     // same module driver command
		0x22fc: "8e06c03326f606ff0580",                   // reticle damage gate follows speech
	} {
		want, e := hex.DecodeString(raw)
		if e != nil || !bytes.Equal(b[0x1dc60+ip:][:len(want)], want) {
			t.Fatalf("original section2:%04x operands", ip)
		}
	}
	if !bytes.Equal(b[0x487f0+0xab3:][:2], []byte{0, 0}) {
		t.Fatal("original initialized AB3 is not zero")
	}
	// The resident Alt+C dispatcher and source flag mutation are independent of
	// CPG18/08/00 duties. The key table uses a byte displacement, not a row ID.
	for at, raw := range map[int]string{
		0x600 + 0x450: "2d0010a9ff017403e9b201c1e8083d1e007603e9a701",
		0x600 + 0x48a: "f005",
		0x600 + 0x5f0: "803ede000074168036de00805e5fc9cb",
	} {
		want, e := hex.DecodeString(raw)
		if e != nil || !bytes.Equal(b[at:][:len(want)], want) {
			t.Fatalf("original Alt+C resident file%04x", at)
		}
	}
}

func TestAcquisitionSpeechSignedBoundariesAndKindDomain(t *testing.T) {
	for _, q := range []struct {
		x    int16
		kind byte
		want uint16
	}{
		{-32768, 4, 0x2d}, {-33, 4, 0x2d}, {-32, 4, 0x30},
		{0, 4, 0x30}, {32, 4, 0x30}, {33, 4, 0x2e}, {32767, 4, 0x2e},
		{-33, 0, 0x45}, {0, 1, 0x48}, {33, 1, 0x46},
		{0, 2, 0x30}, {0, 0xff, 0x30},
	} {
		var s AcquisitionSpeechState
		got, ok := s.Advance(AcquisitionSpeechInput{Reached: true, Target: 2, Symbol: 3, Kind: q.kind, TargetX: q.x, SpeechEnabled: true})
		if !ok || got != q.want || s.PreviousTarget != 2 {
			t.Fatalf("x%d kind%d got%02x/%v want%02x", q.x, q.kind, got, ok, q.want)
		}
	}
}

func TestAcquisitionSpeechReachedPhaseHistoryAndGates(t *testing.T) {
	var s AcquisitionSpeechState
	in := AcquisitionSpeechInput{Target: 2, Symbol: 1, Kind: 4, SpeechEnabled: true}
	if _, ok := s.Advance(in); ok || s.PreviousTarget != 0 {
		t.Fatal("unreached phase changed speech history")
	}
	in.Reached, in.Symbol = true, 0
	if _, ok := s.Advance(in); ok || s.PreviousTarget != 0 {
		t.Fatal("no target symbol changed history")
	}
	in.Symbol, in.SpeechEnabled = 2, false
	if _, ok := s.Advance(in); ok || s.PreviousTarget != 2 {
		t.Fatal("source disabled speech must still consume target change")
	}
	in.SpeechEnabled = true
	if _, ok := s.Advance(in); ok {
		t.Fatal("enabling speech replayed a consumed target")
	}
	in.Target = 0
	s.Advance(in)
	in.Target = 2
	if _, ok := s.Advance(in); ok {
		t.Fatal("same-row reacquisition replayed speech")
	}
	in.Target, in.TargetX = 3, -33
	if command, ok := s.Advance(in); !ok || command != 0x2d {
		t.Fatal("different row failed to announce")
	}
	in.Target = 2
	if _, ok := s.Advance(in); !ok {
		t.Fatal("return after another announced identity must announce")
	}
}
