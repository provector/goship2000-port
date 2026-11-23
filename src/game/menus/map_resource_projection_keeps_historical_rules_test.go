package menus

import (
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	"testing"
)

func TestMapResourceProjectionKeepsHistoricalRules(t *testing.T) {
	for _, tc := range []struct {
		profile, recorded, current string
		want                       bool
	}{
		{replay.R221Compatibility, assets.LogicalPreparation33, assets.LogicalPreparation33, true},
		{replay.R221AuthoredCompatibility, assets.LogicalPreparation32, assets.LogicalPreparation33, true},
		{replay.R22Compatibility, assets.LogicalPreparation32, assets.LogicalPreparation33, true},
		{replay.R21AuthoredCompatibility, assets.LogicalPreparation32, assets.LogicalPreparation33, true},
		{replay.R12Compatibility, assets.LogicalPreparation31, assets.LogicalPreparation33, true},
		{replay.R221Compatibility, assets.LogicalPreparation31, assets.LogicalPreparation33, false},
		{replay.R221Compatibility, assets.LogicalPreparation33, assets.LogicalPreparation32, false},
		{replay.R221Compatibility, "unknown", assets.LogicalPreparation33, false},
		{replay.R221Compatibility, assets.LogicalPreparation33, "unknown", false},
	} {
		f := &replay.Film{Header: replay.Header{Simulation: tc.profile, Resources: tc.recorded}}
		if got := filmResourceCompatible(f, tc.current); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
