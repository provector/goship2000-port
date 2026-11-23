package assets

import (
	"fmt"
	"gunship2000-assets/game/world"
)

func prepareTerrainFeedback(gs *Resource, p *AssetVersionProfile, b *world.Bundle) error {
	if gs == nil || p != &IslandIce469085 || !p.SourceMatches(RoleSimulationExecutable, gs.Source.PayloadSHA256) {
		return fmt.Errorf("terrain feedback requires recognized GS2 identity")
	}
	// Selected VGA section7 original1d47:4000..5fff => file3e550+IP.
	// This static artwork interval excludes the staging palette, local state,
	// all38 source CS relocation words, the retained gap and live DATA alias.
	// Only low2 is retained. Mutable DS is deliberately absent from this policy.
	const start = 0x3e550 + 0x4000
	if start > len(gs.Payload)-world.TerrainFeedbackSamples {
		return fmt.Errorf("terrain feedback source range outside profile")
	}
	raw := gs.Payload[start : start+world.TerrainFeedbackSamples]
	packed := make([]byte, len(raw)/4)
	for i, v := range raw {
		packed[i>>2] |= (v & 3) << (2 * (i & 3))
	}
	m := &world.TerrainFeedback{Policy: world.TerrainFeedbackPolicy, SourceSHA256: gs.Source.PayloadSHA256, SourceFileOffset: start, SourceCSOffset: 0x4000, SourceBytes: len(raw), SourceSliceHash: Hash(raw), PackedSHA256: Hash(packed), PackedLow2: packed}
	if err := m.Initialize(); err != nil {
		return err
	}
	b.TerrainFeedback = m
	b.Notes = append(b.Notes, "Raised PHIL terrain uses native periodic source-CS-low2 span feedback; mutableDS deliberately omitted. Logical320 destination modulo8192 indexes source section7:4000..5fff. Other source-memory materials retain the named hatch fallback; exact supplied carriers remain separate.")
	return nil
}
