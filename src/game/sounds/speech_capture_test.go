package sounds

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/audioopl"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// This is an explicitly supplied audio audition, not an original event trace.
// Fixed effects share retained FM/rotor state, while acquisition lists use the
// source-priority PCM queue. Neither path has a gameplay RNG or render clock.
func TestSpeechCombatAudioDiagnosticCapture(t *testing.T) {
	root := os.Getenv("GS2000_M1_AUDIO_CAPTURE")
	if root == "" {
		t.Skip("set a fresh GS2000_M1_AUDIO_CAPTURE and prepared tree")
	}
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	a, err := assetdata.LoadAudioAssets(os.Getenv("GS2000_TEST_PREPARED"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := rotorSequence(a.ASoundGS2Data)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.rotorCommand(1); err != nil {
		t.Fatal(err)
	}
	type command struct {
		Seconds float64
		Event   uint16
	}
	fm := []command{{.8, 7}, {1, 7}, {1.2, 7}, {2.3, 9}, {2.5, 9}, {3.5, 0x18}, {4, 8}, {4.8, 0x11}, {5.3, 0x10}}
	digital := []command{{.3, 0x30}, {1.5, 0x2d}, {2.7, 0x2e}, {3.9, 0x48}, {5.1, 0x34}, {6.3, 0x3e}}
	next := 0
	for q.pit < uint64(q.clocks.PITFrequency)*8 {
		for next < len(fm) && q.pit >= uint64(fm[next].Seconds*float64(q.clocks.PITFrequency)) {
			if err = q.gs2FixedCommand(a.GS2Commands[fm[next].Event]); err != nil {
				t.Fatal(err)
			}
			next++
		}
		if q.pit >= uint64(q.clocks.PITFrequency)*7 && q.word(q.layout.NoiseArgument) != 0 {
			if err = q.rotorCommand(0); err != nil {
				t.Fatal(err)
			}
		}
		q.interrupt()
		if q.err != nil {
			t.Fatal(q.err)
		}
	}
	frames := q.sampleFrame() + 1
	pcm, err := audioopl.Render(audioFMRate, frames, q.componentWrites())
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewAudioPlayer(a, DefaultAudioPreferences())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	next = 0
	var peak int32
	one := make([]float32, 1)
	for i := uint64(0); i < frames; i++ {
		for next < len(digital) && i >= uint64(digital[next].Seconds*float64(audioFMRate)) {
			if err = p.PlayGS2Event(digital[next].Event, 0); err != nil {
				t.Fatal(err)
			}
			next++
		}
		p.Render(one, 1)
		pcm[i] += int32(math.Round(float64(one[0]) * 32768))
		v := pcm[i]
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	encoded, clips, last := audioPCM16(pcm)
	if err = os.WriteFile(filepath.Join(root, "supplied-rotor-combat-speech.wav"), audioWAV(encoded, audioFMRate, 16), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(struct {
		FM, Digital   []command
		Frames        uint64
		Peak          int32
		Clipped       uint64
		LastAudible   int64
		Qualification string
	}{fm, digital, frames, peak, clips, last, "Supplied eight-second audition; not naturally observed/original timing. Shared retained DBOPL FM/rotor plus exact8000Hz PCM queue at49716Hz. Unity clamp recorded without normalization; no gameplay RNG or render clock."}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "inputs.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
