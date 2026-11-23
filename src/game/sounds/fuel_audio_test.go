package sounds

import (
	"bytes"
	"encoding/binary"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

func TestFuelWarningOriginalCallersAndPreparedPCM(t *testing.T) {
	g, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	// Section1 maps file=d8b0+IP. These literal original operands distinguish
	// old fuel from the post-decrement value, DE80 and the strict height gate.
	for _, tc := range []struct {
		ip  int
		raw []byte
	}{
		{0x4bf, []byte{0x26, 0x83, 0xbf, 0x17, 0, 0, 0x7f, 3}},
		{0x39d, []byte{0x26, 0x8a, 0x84, 3, 6, 0x24, 3, 0xfe, 0xc8}},
		{0x4d7, []byte{0xf6, 6, 0xde, 0, 0x80}},
		{0x4e9, []byte{0x6a, 0x33, 0x9a, 0x0b, 0, 0x44, 8}},
		{0x506, []byte{0x6a, 0x3c, 0x9a, 0x0b, 0, 0x44, 8}},
		{0x3ea, []byte{0x26, 0x83, 0x3e, 0xe7, 0x0a, 2, 0x7e, 0x0f}},
		{0x3f2, []byte{0x6a, 0x3d, 0x9a, 0x0b, 0, 0x44, 8}},
		{0x3fc, []byte{0x80, 0x36, 0xde, 0, 0x80}},
		{0x401, []byte{0xf6, 6, 0xde, 0, 0x20}},
		{0x408, []byte{0x6a, 0, 0x6a, 0x0b}},
	} {
		if !bytes.Equal(g[0xd8b0+tc.ip:][:len(tc.raw)], tc.raw) {
			t.Fatalf("original fuel/audio IP%04x changed", tc.ip)
		}
	}
	s := initializationSource(t)
	for _, tc := range []struct {
		event, handler int
		raw            []byte
	}{
		{0x33, 0xb82, []byte{0xb9, 2, 0, 0x8d, 0x1e, 0x95, 0x0c}},
		{0x3c, 0xc22, []byte{0xb9, 8, 0, 0x8d, 0x1e, 0x75, 0x0d}},
	} {
		if audioWord(s, 512+0x104f+2*tc.event) != uint16(tc.handler) || !bytes.Equal(s[512+tc.handler:][:len(tc.raw)], tc.raw) {
			t.Fatalf("original ASOUND event%02x/priority/descriptor", tc.event)
		}
	}
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("set separate complete prepared22 tree for actual warning PCM")
	}
	a, err := assetdata.LoadAudioAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	bank, err := os.ReadFile(testfixture.Path("installation/TARGET.BIN"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		event      uint16
		priority   int
		start, end int
	}{
		{0x33, 2, 0x625b, 0x722d},
		{0x3c, 8, 0xe9b4, 0xffd4},
	} {
		entry, ok := a.GS2Digital[tc.event]
		if !ok || entry.Priority != tc.priority || entry.Clip == nil {
			t.Fatalf("prepared fuel event%02x priority%d unavailable", tc.event, tc.priority)
		}
		// Original descriptors0c95/0d75 share first range5437..6259. Source
		// DMA count rounds each EndWord-StartWord down to even, not inclusive.
		want := append(append([]byte{}, bank[0x5437:0x6259]...), bank[tc.start:tc.end]...)
		if len(entry.Clip.Samples) != len(want) {
			t.Fatalf("event%02x logical assembly length", tc.event)
		}
		for i, b := range want {
			if entry.Clip.Samples[i] != int16((int(b)-128)<<8) {
				t.Fatalf("event%02x source PCM sample%d", tc.event, i)
			}
		}
	}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	for _, event := range []uint16{0x33, 0x3c, 0x3d} {
		if err := p.PlayGS2Event(event, 0); err != nil {
			t.Fatal(err)
		}
	}
	if p.digitalCurrent.event != 0x33 || p.digitalPending[0].event != 0x3d || p.digitalPending[1].event != 0x3c {
		t.Fatal("fuel warnings bypassed source nonpreemptive active/priority-sorted pending lists")
	}
	block := make([]float32, 4096)
	p.SetEffectsPaused(true)
	p.Render(block, len(block))
	if p.digitalFrames != 0 {
		t.Fatal("pause/GPS advanced fuel speech")
	}
	p.SetEffectsPaused(false)
	var audible bool
	for i := 0; i < 12; i++ {
		p.Render(block, len(block))
		for _, v := range block {
			audible = audible || v != 0
		}
	}
	if !audible {
		t.Fatal("prepared source warning produced silent PCM")
	}
	// Direct descriptor words are asserted separately from prepared output.
	if binary.LittleEndian.Uint16(s[512+0xc95:]) != 0x625a || binary.LittleEndian.Uint16(s[512+0xd75+6:]) != 0xffd4 {
		t.Fatal("fuel speech descriptor boundary changed")
	}
}
