package flight

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSelectedStationSourceLoop(t *testing.T) {
	// Original loop increments before checking; compact1 is not skipped.
	ids := [4]byte{2, 0, 1, 7}
	current := 0
	for _, want := range []int{2, 3, 0, 2} {
		got, err := NextSelectedStation(current, ids)
		if err != nil || got != want {
			t.Fatalf("from%d want%d got%d: %v", current, want, got, err)
		}
		current = got
	}
	if got, err := NextSelectedStation(3, [4]byte{0, 0, 0, 1}); err != nil || got != 3 {
		t.Fatalf("sole EMPTY entry must remain selectable: %d %v", got, err)
	}
	if got, err := NextSelectedStation(0, [4]byte{}); err == nil || got != 0 {
		t.Fatal("all-NO-BAY malformed state must return useful error")
	}
	for _, current := range []int{-1, 4, 256} {
		if got, err := NextSelectedStation(current, ids); err == nil || got != current {
			t.Fatal("invalid incoming station must not be normalized")
		}
	}
}

func TestSelectedStationOriginalOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional selected source installation unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ip  int
		hex string
	}{
		{0xfa8, "26ff06d30a26833ed30a037e07"},
		{0xfb5, "26c706d30a0000"},
		{0xfc5, "2680bf01000074d7"},
		//3900h is the preceding jettison arm;3920h reaches selection.
		{0x138e, "2d20007503e90efc"},
	} {
		want, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatal(err)
		}
		at := 0xd8b0 + tc.ip
		if !bytes.Equal(raw[at:at+len(want)], want) {
			t.Fatalf("selected source mismatch section1:%04x", tc.ip)
		}
	}
}
