package avionics

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestTargetMetricUnsignedWordBoundary(t *testing.T) {
	for _, tc := range []struct {
		x, z int16
		want uint16
	}{
		{0, 0, 0}, {4, 6, 8}, {-4, -6, 8},
		{-32768, 2, 32769}, {2, -32768, 32769},
		{-32768, -32768, 49152}, {32767, 32767, 49150},
	} {
		if got := TargetMetric(tc.x, tc.z); got != tc.want {
			t.Fatalf("target display unsigned ABS/max (%d,%d): want%d got%d", tc.x, tc.z, tc.want, got)
		}
	}
	if Metric(-32768, 2) != 49154 {
		t.Fatal("the separate signed contact metric must retain its source behavior")
	}
}

func TestTargetMetricOriginalUnsignedComparison(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional selected source installation unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("3bfe76098bc6d1e803c7eb0590d1e803c6c1e805")
	// Resident00db uses file13b0+IP, not the section1 coordinate formula.
	at := 0x13b0 + 0x183
	if !bytes.Equal(raw[at:at+len(want)], want) {
		t.Fatal("target range JBE/SHR/ADD and finalSHR5 original operands differ")
	}
}
