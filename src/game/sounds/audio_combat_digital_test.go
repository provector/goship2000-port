package sounds

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	assetdata "gunship2000-assets/game/assets"
)

func TestGS2DigitalQueueOriginalOperandsAndPriority(t *testing.T) {
	s := initializationSource(t)
	for _, v := range []struct {
		at  int
		hex string
	}{
		{0xb8c, "b903008d1ea30ce91b02"}, // event34: priority3, descriptor0ca3, jump0db1
		{0xbdc, "b905008d1e130de9cb01"}, // primary count-kind3 completion: priority5, descriptor0d13
		{0xbe6, "b904008d1e210de9c101"}, // secondary count-kind3 completion: priority4, descriptor0d21
		{0xdbe, "2e3b0e870d7f1c"},       // signed strictlygreater than first pending
		{0xdcd, "2e3b0e890d7e32"},       // <=second discards incoming
		{0xe0f, "2e833eaf0d007556"},     // separate activebusy; no current preemption
	} {
		b, e := hex.DecodeString(v.hex)
		if e != nil || !bytes.Equal(s[512+v.at:512+v.at+len(b)], b) {
			t.Fatalf("source CS%04x changed", v.at)
		}
	}
	p := &AudioPlayer{}
	asset := func(priority int) assetdata.AudioDigitalPlayback {
		return assetdata.AudioDigitalPlayback{Priority: priority, Clip: &assetdata.AudioClip{Samples: []int16{1000}, SampleRate: 8000}}
	}
	p.queueGS2Digital(1, asset(2)) // immediately active
	p.queueGS2Digital(2, asset(1))
	p.queueGS2Digital(3, asset(4)) // shifts2 into second
	p.queueGS2Digital(4, asset(3)) // replaces only second
	p.queueGS2Digital(5, asset(3)) // equality cannot replace second
	if p.digitalCurrent.event != 1 || p.digitalPending[0].event != 3 || p.digitalPending[1].event != 4 {
		t.Fatal("source pending priority or current preemption changed")
	}
	p.startGS2Digital()
	if p.digitalCurrent.event != 3 || p.digitalPending[0].event != 4 || p.digitalPending[1] != nil {
		t.Fatal("source list advance ordering changed")
	}
}

func TestGS2CountObjectiveDigitalOriginalCallersAndAssemblies(t *testing.T) {
	s := initializationSource(t)
	g, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		ip  int
		raw string
	}{{0x6398, "f606de0080740a6a3e9a0b00440883c402"}, {0x6b06, "f606de0080740a6a3f9a0b00440883c402"}} {
		raw, _ := hex.DecodeString(v.raw)
		if !bytes.Equal(g[0xd8b0+v.ip:][:len(raw)], raw) {
			t.Fatalf("original section1 speech guard/call%04x", v.ip)
		}
	}
	for _, v := range []struct {
		event, handler, descriptor int
		raw                        string
	}{{0x3e, 0xbdc, 0xd13, "9addb9d6000064c181ac00000000"}, {0x3f, 0xbe6, 0xd21, "9addb9d60000b8d665c100000000"}} {
		if audioWord(s, 512+0x104f+v.event*2) != uint16(v.handler) {
			t.Fatal("original event table", v)
		}
		raw, _ := hex.DecodeString(v.raw)
		if !bytes.Equal(s[512+v.descriptor:][:len(raw)], raw) {
			t.Fatal("original CS-relative two-block descriptor/sentinel", v)
		}
	}
	if !reflect.DeepEqual(assetdata.IslandIce469085.AudioLayout.Drivers["GS2"].DigitalGameplayEvents, []int{0x2d, 0x2e, 0x30, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x45, 0x46, 0x48}) {
		t.Fatal("bounded digital event admission domain")
	}
}

func TestGS2CountObjectivePreparedPlaybackAndPriority(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("set separate prepared asset tree")
	}
	a, err := assetdata.LoadAudioAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	bank, err := os.ReadFile(filepath.Join(testfixture.Path("installation"), "TARGET.BIN"))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		event      uint16
		priority   int
		start, end int
	}{{0x3e, 5, 0xac81, 0xc163}, {0x3f, 4, 0xc165, 0xd6b7}} {
		entry, ok := a.GS2Digital[v.event]
		if !ok || entry.Priority != v.priority {
			t.Fatal("prepared objective speech event/priority", v)
		}
		// Independent original descriptor operands give the same1760-byte
		// leading block, followed by5346 or5458 bytes. EndWord-start is rounded
		// down even by the source DMA count convention, not inclusive length.
		want := append(append([]byte{}, bank[0xd6b9:0xdd99]...), bank[v.start:v.end]...)
		if len(entry.Clip.Samples) != len(want) {
			t.Fatal("logical source assembly length", v, len(entry.Clip.Samples), len(want))
		}
		for i, b := range want {
			if entry.Clip.Samples[i] != int16((int(b)-128)<<8) {
				t.Fatalf("source PCM assembly event%02x sample%d", v.event, i)
			}
		}
	}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []uint16{0x34, 0x3f, 0x3e, 0x34} {
		if err = p.PlayGS2Event(e, 0); err != nil {
			t.Fatal(err)
		}
	}
	if p.digitalCurrent.event != 0x34 || p.digitalPending[0].event != 0x3e || p.digitalPending[1].event != 0x3f {
		t.Fatal("actual admitted commands bypassed source pending priority/nonpreemption")
	}
	if err = p.PlayGS2Event(0x3e, 1); err == nil {
		t.Fatal("unestablished digital argument accepted")
	}
}

func TestGS2Digital8000HoldPauseMuteAndExit(t *testing.T) {
	clip := &assetdata.AudioClip{Samples: []int16{1000, 2000, 3000}, SampleRate: 8000}
	a := &assetdata.AudioAssets{Clips: map[string]*assetdata.AudioClip{}, GS2Digital: map[uint16]assetdata.AudioDigitalPlayback{0x34: {Clip: clip, Priority: 3}}}
	p, e := NewAudioPlayer(a, DefaultAudioPreferences())
	if e != nil {
		t.Fatal(e)
	}
	if e = p.PlayGS2Event(0x34, 0); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 19; i++ {
		expect := float64(1000)
		if i >= 7 {
			expect = 2000
		}
		if i >= 13 {
			expect = 3000
		}
		if v := p.gs2DigitalSample(); v != expect {
			t.Fatalf("floor(%d*8000/49716) sample%v want%v", i, v, expect)
		}
	}
	if v := p.gs2DigitalSample(); v != 0 || p.digitalCurrent != nil {
		t.Fatal("digital end did not release list")
	}
	p.PlayGS2Event(0x34, 0)
	p.SetEffectsPaused(true)
	b := make([]float32, 10)
	p.Render(b, 10)
	if p.digitalFrames != 0 || !reflect.DeepEqual(b, make([]float32, 10)) {
		t.Fatal("GPS/pause advanced digital playback")
	}
	p.SetEffectsPaused(false)
	p.SetSoundEffectsEnabled(false)
	p.Render(b, 10)
	if p.digitalFrames != 10 || !reflect.DeepEqual(b, make([]float32, 10)) {
		t.Fatal("mute must suppress output and retain current phase")
	}
	p.SetSoundEffectsEnabled(true)
	p.prefs.EffectsVolume = 0
	p.Render(b, 3)
	if p.digitalFrames != 13 || !reflect.DeepEqual(b, make([]float32, 10)) {
		t.Fatal("volume0 did not retain phase/silence")
	}
	p.PlayGS2Event(0x34, 0)
	p.StopRotor()
	if p.digitalCurrent != nil || p.digitalPending != ([2]*gs2DigitalVoice{}) {
		t.Fatal("flight digital voice leaked into briefing")
	}
}

// Step40/41 trace these ASOUND digital commands from incoming contact/quadrant
// consumers; numeric similarity to GS menu cues is not an admission criterion.
func TestIncomingDigitalOriginalConstructors(t *testing.T) {
	s := initializationSource(t)
	for _, q := range []struct {
		event, handler int
		raw            string
	}{
		{0x35, 0xb96, "b901008d1eb10ce91102"}, {0x36, 0xba0, "b901008d1ebf0ce90702"},
		{0x37, 0xbaa, "b901008d1ecd0ce9fd01"}, {0x38, 0xbb4, "b901008d1edb0ce9f301"},
		{0x3b, 0xbc8, "b901008d1ef70ce9df01"},
	} {
		b, _ := hex.DecodeString(q.raw)
		if audioWord(s, 512+0x104f+q.event*2) != uint16(q.handler) || !bytes.Equal(s[512+q.handler:][:len(b)], b) {
			t.Fatalf("incoming ASOUND%02x constructor changed", q.event)
		}
	}
}
