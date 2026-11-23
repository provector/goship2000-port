package flight

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestHighWarningBodyUnitsPriorityAndReachedRepetition(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	for ip, h := range map[int]string{0x50: "a1a03b48488946fe", 0x7f8: "6b46fe19c1e805"} {
		want, _ := hex.DecodeString(h)
		if !bytes.Equal(b[0x1dc60+ip:][:len(want)], want) {
			t.Fatalf("source body/dial arithmetic%x differs", ip)
		}
	}
	// Recovered source dial conversion is (max(H-2,0)*25)>>5 with word
	// multiplication. The warning reads H directly, not this display or mast.
	display := func(h uint16) uint16 {
		if h < 2 {
			return 0
		}
		return uint16((h-2)*25) >> 5
	}
	if display(1500) != 1170 || display(1501) != 1171 || display(1920) != 1498 {
		t.Fatal("source world/display separation")
	}
	for _, tc := range []struct {
		h, terrain, low, v int16
		want               byte
	}{
		{1499, 0, 128, 1, 0}, {1500, 0, 128, 1, 0}, {1501, 0, 128, 1, 5},
		{1501, 0, 128, 0, 0}, {1501, 0, 128, -1, 0}, {1921, 0, 128, 1, 5},
		{1501, 1490, 128, -1, 4}, {1501, 1490, 128, 1, 5},
		{1501, 1499, 128, -1, 0},
	} {
		for repeat := 0; repeat < 3; repeat++ {
			if got := AltitudeWarning(tc.h, tc.terrain, tc.low, tc.v, 0x40); got != tc.want {
				t.Fatal(tc, got)
			}
		}
	}
	// Vertical's sign is positive up. No fabricated cooldown/ceiling warning
	// on level or falling flight, nor camera+128 substitution, is authorized.
	if AltitudeWarning(1400, 0, 128, 1, 0x40) != 0 || AltitudeWarning(1528, 0, 128, 1, 0x40) != 5 {
		t.Fatal("body and hypothetical mast height were conflated")
	}
}
