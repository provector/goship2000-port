package assets

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestSMPOriginalFraming(t *testing.T) {
	raw := make([]byte, 4+89*65)
	binary.LittleEndian.PutUint16(raw, 89)
	binary.LittleEndian.PutUint16(raw[2:], 65)
	for i := 4; i < len(raw); i++ {
		raw[i] = byte(i)
	}
	v, e := DecodeOverview(raw)
	if e != nil || !bytes.Equal(v, raw[4:]) {
		t.Fatal("raw indices changed", e)
	}
	v[0] ^= 255
	if v[0] == raw[4] {
		t.Fatal("outputaliasesoriginal")
	}
	for _, bad := range [][]byte{raw[:len(raw)-1], append(append([]byte{}, raw...), 0), bytes.Repeat([]byte{0xf5}, len(raw))} {
		if _, e = DecodeOverview(bad); e == nil {
			t.Fatal("badheaderorlengthaccepted")
		}
	}
}

func TestSMPAllSourceCarriers(t *testing.T) {
	root := testfixture.Path("installation")
	for _, name := range []string{"ANTC1", "ANTC2", "ANTC3", "EURO6", "EURO7", "EURO8", "EURO9", "GULF2", "GULF3", "GULF4", "GULF5", "PHIL1", "PHIL2", "PHIL3"} {
		raw, e := os.ReadFile(filepath.Join(root, name+"SMP.GS2"))
		if e != nil {
			t.Fatal(e)
		}
		v, e := DecodeOverview(raw)
		if e != nil || len(v) != 5785 || !bytes.Equal(v, raw[4:]) {
			t.Fatalf("%s:%v", name, e)
		}
	}
}
