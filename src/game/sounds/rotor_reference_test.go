package sounds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/audioopl"
)

func TestRotorStatefulReferenceCapture(t *testing.T) {
	dir := os.Getenv("GS2000_L2_ROTOR_REFERENCE")
	if dir == "" {
		t.Skip("optional deterministic listening/reference export")
	}
	src := initializationSource(t)
	l := assetdata.IslandIce469085.AudioLayout
	q, e := rotorSequence(src[l.Drivers["GS2"].DSFile:])
	if e != nil {
		t.Fatal(e)
	}
	events := []struct {
		At       uint64
		Argument uint16
	}{{0, 1}, {2 * 1193182, 15}, {3 * 1193182, 2}, {4 * 1193182, 0}, {7 * 1193182, 1}}
	index := 0
	var witness []map[string]any
	for {
		for index < len(events) && events[index].At <= q.pit {
			v := events[index]
			if e = q.rotorCommand(v.Argument); e != nil {
				t.Fatal(e)
			}
			witness = append(witness, map[string]any{"requested_pit": v.At, "applied_pit": q.pit, "argument": v.Argument, "speed": q.word(q.layout.NoiseSpeed), "step": int16(q.word(q.layout.NoiseStep)), "level": q.mem[q.layout.NoiseLevel]})
			index++
		}
		if q.pit >= 10*1193182 {
			break
		}
		q.interrupt()
		if q.err != nil {
			t.Fatal(q.err)
		}
	}
	frames := q.sampleFrame()
	writes := make([]audioopl.Write, len(q.writes))
	for i, w := range q.writes {
		writes[i] = audioopl.Write{Frame: w.PIT * audioFMRate / 1193182, Register: uint16(w.Register), Value: w.Value}
	}
	pcm, e := audioopl.Render(audioFMRate, frames, writes)
	if e != nil {
		t.Fatal(e)
	}
	stored, clips, last := audioPCM16(pcm)
	if clips != 0 {
		t.Fatalf("source gain1 clips%d", clips)
	}
	if e = os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	write := func(name string, b []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(b); e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
	}
	write("rotor-history.wav", audioWAV(stored, audioFMRate, 16))
	doc, _ := json.MarshalIndent(map[string]any{"policy": RotorPolicy, "profile": assetdata.IslandIce469085.Name, "source_sha256": assetdata.Hash(src), "sample_rate": audioFMRate, "frames": frames, "output_gain": 1, "clipped_samples": clips, "last_nonzero_frame": last, "events": witness, "register_writes": len(writes), "scope": "Source-derived deterministic listening vector; commands applied on next source IRQ for this diagnostic only. Batch uses same retained core; not original audio or a natural flight run."}, "", "  ")
	write("rotor-history.json", doc)
}
