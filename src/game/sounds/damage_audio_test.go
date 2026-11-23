package sounds

import (
	"bytes"
	"encoding/binary"
	assetdata "gunship2000-assets/game/assets"
	"os"
	"testing"
)

func TestDamageAndTerminalAudioSourceDomains(t *testing.T) {
	s := initializationSource(t)
	if binary.LittleEndian.Uint16(s[512+0x104f+0x28*2:]) != 0xa26 || !bytes.Equal(s[512+0xa26:512+0xa2d], []byte{0x8d, 0x0e, 0x5c, 9, 0xe9, 0xd5, 0xfc}) {
		t.Fatal("GS2 terminal28 source constructor")
	}
	l := assetdata.IslandIce469085.AudioLayout.Drivers["GS2"]
	c, err := prepareGS2Command(s, l, assetdata.IslandIce469085.AudioLayout, 0x28)
	if err != nil || c.Channel != 0 || c.Start != 0x95c || c.Alternate != 0 {
		t.Fatal("source28 row/stream", c, err)
	}
	if binary.LittleEndian.Uint16(s[512+0x104f+0x3d*2:]) != 0xbd2 || !bytes.Equal(s[512+0xbd2:512+0xbd9], []byte{0xb9, 10, 0, 0x8d, 0x1e, 5, 13}) {
		t.Fatal("GS2 warning3d digital priority/descriptor")
	}
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("set separate complete prepared21 tree for playback metadata")
	}
	a, err := assetdata.LoadAudioAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	if a.GS2Digital[0x3d].Priority != 10 || a.GS2Digital[0x3d].Clip == nil || a.GS2Commands[0x28].Start != 0x95c {
		t.Fatal("prepared source audio admission")
	}
	// Exercise the actual prepared warning, not just command admission or a
	// backend-ready flag. The digital cursor resets on completion, so an end-of-
	// window status of zero frames alone does not mean the cue was silent.
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	if err := p.PlayGS2Event(0x3d, 0); err != nil {
		t.Fatal(err)
	}
	p.SetEffectsPaused(true)
	block := make([]float32, 4096)
	p.Render(block, len(block))
	if p.digitalFrames != 0 {
		t.Fatal("paused warning advanced")
	}
	p.SetEffectsPaused(false)
	nonzero := false
	for i := 0; i < 8; i++ {
		p.Render(block, len(block))
		for _, v := range block {
			nonzero = nonzero || v != 0
		}
	}
	if !nonzero {
		t.Fatal("prepared damage warning produced no PCM")
	}
}
