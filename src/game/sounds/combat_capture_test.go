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

func TestCombatAudioDiagnosticCapture(t *testing.T) {
	root := os.Getenv("GS2000_M_AUDIO_CAPTURE")
	if root == "" {
		t.Skip("set a fresh GS2000_M_AUDIO_CAPTURE and prepared audio tree")
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
	type request struct {
		Seconds float64
		Event   uint16
	}
	events := []request{{.8, 7}, {1, 7}, {1.2, 7}, {1.6, 9}, {1.8, 9}, {2.3, 0x18}, {2.8, 8}, {3.5, 0x11}, {4, 0x10}, {4.7, 0x25}, {5.4, 0x27}, {6, 0x4a}, {6.6, 3}}
	next := 0
	for q.pit < uint64(q.clocks.PITFrequency)*8 {
		for next < len(events) && q.pit >= uint64(events[next].Seconds*float64(q.clocks.PITFrequency)) {
			if err = q.gs2FixedCommand(a.GS2Commands[events[next].Event]); err != nil {
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
	start := uint64(42 * audioFMRate / 10)
	var peak int32
	for i := start; i < frames; i++ {
		if i == start {
			if err = p.PlayGS2Event(0x34, 0); err != nil {
				t.Fatal(err)
			}
		}
		sample := make([]float32, 1)
		p.Render(sample, 1)
		pcm[i] += int32(math.Round(float64(sample[0]) * 32768))
	}
	for _, v := range pcm {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	encoded, clips, last := audioPCM16(pcm)
	// Runtime's existing final mixer clamps FM+SB at unity. Record those rare
	// mixed-output clips; do not normalize source WAVs or conceal the limit.
	if err = os.WriteFile(filepath.Join(root, "rotor-combat-history.wav"), audioWAV(encoded, audioFMRate, 16), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(struct {
		Policy        string
		Events        []request
		Frames        uint64
		Peak          int32
		Last          int64
		Clips         uint64
		Qualification string
	}{"gs2-shared-fixed-rows-dbopl-v1 + logical8000Hz speech queue", events, frames, peak, last, clips, "Explicit synthetic audition schedule, not original gameplay timing or a natural firing trace. Private audio RNG only. Uses prepared typed rows/data and existing pinned DBOPL; source command tables and original instructions independently checked elsewhere."}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "inputs.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
