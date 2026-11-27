package replay

import (
	"bytes"
	"github.com/klauspost/compress/s2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExperimentalRowsAndLimits(t *testing.T) {
	for _, events := range [][]Event{nil, {}, {{Index: 1 << 60, Kind: 255, Actor: 4, Target: 3, Timestamp: 65535}}} {
		op := blocksOp(1)
		op.Checkpoint = map[string]string{"flight": strings.Repeat("ab", 32), "visual": strings.Repeat("01", 32)}
		op.Events = events
		row, e := protoRow(op)
		if e != nil {
			t.Fatal(e)
		}
		rows, e := decodeProtoRows(row, 1)
		if e != nil || len(rows) != 1 || !reflect.DeepEqual(op, rows[0]) {
			t.Fatal("roundtrip", e)
		}
		for _, raw := range [][]byte{row[:len(row)-1], append(bytes.Clone(row), row...)} {
			if _, e = decodeProtoRows(raw, 1); e == nil {
				t.Fatal("invalid row stream accepted")
			}
		}
	}
	if boundedProtoOperation(bytes.Repeat([]byte{0x1a, 0}, 33)) || boundedProtoOperation(bytes.Repeat([]byte{0x32, 0}, 65537)) {
		t.Fatal("excessive nested message count")
	}
	bad := blocksOp(1)
	bad.Checkpoint = map[string]string{"new-domain": strings.Repeat("aa", 32)}
	if _, err := protoRow(bad); err == nil {
		t.Fatal("unrecognized domain silently dropped")
	}
	if _, e := decodeS2(s2.Encode(nil, []byte("sample")), 2); e == nil {
		t.Fatal("decoded length mismatch accepted")
	}
}
func TestExperimentalConversionDoesNotReplaceDestination(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "protected.g2r")
	os.WriteFile(dst, []byte("owned"), 0600)
	// Reuse an ordinary complete film as the source so the failure occurs at publication.
	f := testFilm("00112233445566778899aabbccddeeff")
	raw, e := Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(t.TempDir(), "film.g2r")
	if e = os.WriteFile(src, raw, 0600); e != nil {
		t.Fatal(e)
	}
	{
		good := filepath.Join(t.TempDir(), "example.g2r")
		if e = ConvertContainer(src, good, currentFormat()); e != nil {
			t.Fatal(e)
		}
		if e = ConvertContainer(src, dst, currentFormat()); e == nil {
			t.Fatal("published over existing file")
		}
		b, _ := os.ReadFile(dst)
		if string(b) != "owned" {
			t.Fatal("destination changed")
		}
		blocks, ops, _, _ := InspectPrefix(good)
		if blocks == 0 || ops != f.OperationCount() {
			t.Fatal("complete contiguous prefix", blocks, ops)
		}
		p, _ := os.ReadFile(good)
		for _, cut := range []int{12, len(p) - 1} {
			if e = os.WriteFile(good, p[:cut], 0600); e != nil {
				t.Fatal(e)
			}
			if _, e = OpenFile(good); e == nil {
				t.Fatal("accepted truncated experiment")
			}
		}
	}
}
