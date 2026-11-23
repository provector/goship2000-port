package cockpit

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func alternateDefinition() Definition {
	d := Definition{}
	for off, v := range map[int]int16{0x20: 10, 0x22: 20, 0x28: 40, 0x2a: 20, 0x30: 70, 0x32: 20, 0x4c: 160, 0x4e: 160} {
		d.Parameters[off/2] = v
	}
	return d
}

func TestAlternateIndicatorsHandRegionsAndExactMatching(t *testing.T) {
	d := alternateDefinition()
	frame := bytes.Repeat([]byte{0x55}, 64000)
	fill(frame, 10, 20, 16, 7, 0x47, Full)
	fill(frame, 40, 20, 17, 7, 0xef, Full)
	fill(frame, 70, 20, 5, 5, 0x45, Full)
	frame[20*320+10], frame[20*320+40], frame[20*320+70] = 0x46, 0xee, 0x44
	drawAlternateIndicators(frame, d, Inputs{SystemFlags: 1 | 2, Lateral: 1})
	// The source selects bay/open + gear/down and positive-VSI top3rows.
	for _, tc := range []struct {
		x, y int
		want byte
	}{{10, 20, 0x46}, {11, 20, 0x1f}, {25, 26, 0x1f}, {26, 26, 0x55}, {40, 20, 0xee}, {41, 20, 0x96}, {70, 20, 0x44}, {71, 20, 0x0a}, {71, 22, 0x0a}, {71, 23, 0x45}} {
		if got := frame[tc.y*320+tc.x]; got != tc.want {
			t.Fatalf("pixel%d,%d got%02x want%02x", tc.x, tc.y, got, tc.want)
		}
	}
	for _, tc := range []struct {
		clearance int16
		phase     uint16
		drawn     bool
	}{{64, 1, false}, {63, 0, false}, {63, 1, true}} {
		f := bytes.Repeat([]byte{0xef}, 64000)
		drawAlternateIndicators(f, d, Inputs{SystemFlags: 1 | 4, HUDScale: tc.clearance, BlinkPhase: tc.phase})
		if (f[20*320+40] == 0x96) != tc.drawn {
			t.Fatalf("source lowclearance/phase boundary%+v", tc)
		}
	}
}

func TestAlternateAttitudeCenteredCarrierAndNoMutation(t *testing.T) {
	a := testAssets()
	d := alternateDefinition()
	d.SpanReplacement = 0xffff // marker palette values are separate from attitude match
	i := Image{Width: 31, Height: 31, Pixels: make([]byte, 31*31)}
	i.Pixels[0], i.Pixels[len(i.Pixels)-1] = 12, 14
	a.Sprites["attitude-overlay"] = i
	frame := bytes.Repeat([]byte{0x55}, 64000)
	if err := a.alternateAttitude(frame, d, [3]int16{}); err != nil {
		t.Fatal(err)
	}
	if frame[145*320+145] != 12 || frame[175*320+175] != 14 || frame[160*320+160] != 0x55 {
		t.Fatal("flags3 center or00 preserving failed")
	}
	if i.Pixels[0] != 12 || i.Pixels[len(i.Pixels)-1] != 14 {
		t.Fatal("shared sprite mutated")
	}
}

func TestAlternateInstrumentSourceOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file int
		hex  string
	}{
		{0x1dc60 + 0x85f, "f606e100017403e9b701"},
		{0x1dc60 + 0xa31, "6a1f6a476a076a10"},
		{0x1dc60 + 0xa56, "8e06dc3326833ee70a407d27f606da0001"},
		{0x1dc60 + 0xa69, "68960068ef006a076a11"},
		{0x1dc60 + 0xa9f, "6a0a6a456a036a05"},
		{0x1dc60 + 0x17c4, "f606e100017467"},
		{0x1dc60 + 0x17f3, "6a038e06be3326ff36fa1b26ff36f81b"},
		{0x3e550 + 0xfe7, "f7460e010074088b46fed1e829460a"},
		{0x3e550 + 0x68b, "268a053a460e75038a4610aa"},
	} {
		want, err := hex.DecodeString(tc.hex)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw[tc.file:tc.file+len(want)], want) {
			t.Fatalf("source instrument instruction mismatch file%x", tc.file)
		}
	}
}
