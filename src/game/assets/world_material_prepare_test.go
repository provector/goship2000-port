package assets

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestTerrainFeedbackSourcePreparationAndPreparedOnlyReload(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional recognized source installation")
	}
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(raw)
	gs := &Resource{Source: SourceLocation{PayloadSHA256: Hash(raw)}, Payload: raw}
	b := &world.Bundle{}
	if err := prepareTerrainFeedback(gs, &IslandIce469085, b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatal("preparation modified source bytes")
	}
	m := b.TerrainFeedback
	if !m.Ready() || len(m.PackedLow2) != 2048 {
		t.Fatal("lookup readiness/size")
	}
	for i := 0; i < world.TerrainFeedbackSamples; i++ {
		if m.Low2(i) != raw[0x42550+i]&3 {
			t.Fatalf("source low2 sample%d", i)
		}
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded world.TerrainFeedback
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Ready() {
		t.Fatal("unvalidated prepared metadata ready")
	}
	if err := reloaded.Initialize(); err != nil {
		t.Fatal(err)
	}
	// Runtime lookup is immutable even if a caller edits metadata afterwards.
	old := reloaded.Low2(0)
	reloaded.PackedLow2[0] ^= 3
	if reloaded.Low2(0) != old {
		t.Fatal("drawing reads mutable metadata")
	}
	if err := reloaded.Initialize(); err == nil {
		t.Fatal("corrupt packed payload accepted")
	}
	if reloaded.Ready() {
		t.Fatal("failed validation retained lookup readiness")
	}
	for _, change := range []func(*world.TerrainFeedback){
		func(m *world.TerrainFeedback) { m.Policy = "unknown" },
		func(m *world.TerrainFeedback) { m.SourceFileOffset++ },
		func(m *world.TerrainFeedback) { m.SourceCSOffset++ },
		func(m *world.TerrainFeedback) { m.SourceBytes *= 2 },
		func(m *world.TerrainFeedback) { m.PackedLow2 = m.PackedLow2[:1] },
	} {
		var bad world.TerrainFeedback
		if err := json.Unmarshal(encoded, &bad); err != nil {
			t.Fatal(err)
		}
		change(&bad)
		if err := bad.Initialize(); err == nil || bad.Ready() {
			t.Fatal("unsupported prepared mapping admitted")
		}
	}
	gs.Source.PayloadSHA256 = Hash([]byte("unsupported"))
	if err := prepareTerrainFeedback(gs, &IslandIce469085, &world.Bundle{}); err == nil {
		t.Fatal("unknown executable borrowed source offsets")
	}
}
