package menus

import (
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func TestEditorShadowPerformanceAndIdentity(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	opts, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 0, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	o := opts[0]
	found := false
	for _, candidate := range opts {
		if !candidate.Fixed && candidate.Cost == 1 {
			o = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no movable source singleton")
	}
	e.Context, err = gamestate.NewAuthoredEditorContext(c.editorData(), e.Document.World)
	if err != nil {
		t.Fatal(err)
	}
	results := []map[string]any{}
	candidate := 0
	for _, count := range []int{11, 24, 65} {
		// Source-valid accepted single descriptors through the same placement
		// consumer; no precompiled output, goals or flight state are supplied.
		for len(e.Document.Placements) < count {
			placed := false
			for candidate < 4096 && !placed {
				x := ((candidate % 64) * 24) + 12
				y := (candidate/64)*18 + 9
				candidate++
				if gamestate.PlaceAuthoredOption(c.editorData(), e.Document, "defender", 0, o, x, y) == nil {
					placed = true
				}
			}
			if !placed {
				t.Fatalf("cannot build%d source-legal rows", count)
			}
		}
		e.Role, e.Kind, e.Category, e.Held, e.Options, e.Option = "defender", 0, o.Category, true, []gamestate.AuthoredOption{o}, 0
		e.CursorX, e.CursorY = 120, 100
		c.updateEditorShadow()
		if e.Shadow == nil {
			t.Fatal("benchmark must reach real shadow", e.ShadowError)
		}
		before := gamestate.AuthoredRevision(e.Document)
		bench := testing.Benchmark(func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				e.CursorX = 120 + n%2
				c.updateEditorShadow()
			}
		})
		if gamestate.AuthoredRevision(e.Document) != before {
			t.Fatal("hover mutated descriptors/RNG")
		}
		if bench.AllocedBytesPerOp() > 512 || bench.NsPerOp() > 5_000_000 {
			t.Fatalf("shadow regression at%d: %s / %s", count, bench.String(), bench.MemString())
		}
		results = append(results, map[string]any{"placements": count, "ns_per_update": bench.NsPerOp(), "bytes_per_update": bench.AllocedBytesPerOp(), "allocations_per_update": bench.AllocsPerOp(), "revision_unchanged": true, "scope": "source-valid descriptor fixture; real updateEditorShadow consumer; no OS latency claim"})
		t.Logf("placements%d: %s %s", count, bench.String(), bench.MemString())
	}
	if out := os.Getenv("GS2000_P4_EDITOR_ARTIFACTS"); out != "" {
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(results, "", "  ")
		if err = os.WriteFile(filepath.Join(out, "shadow-performance.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
