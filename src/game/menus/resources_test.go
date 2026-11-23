package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	"testing"
)

func TestExactResourceCapabilitiesAndHistoricalProjection(t *testing.T) {
	for _, tc := range []struct {
		profile, recorded, current string
		want                       bool
	}{
		{replay.R21Compatibility, assetdata.LogicalPreparation32, assetdata.LogicalPreparation32, true},
		{replay.R21Compatibility, assetdata.LogicalPreparation32Candidate, assetdata.LogicalPreparation32, true},
		{replay.R21Compatibility, assetdata.LogicalPreparation32, assetdata.LogicalPreparation31, false},
		{replay.R21Compatibility, assetdata.LogicalPreparation31, assetdata.LogicalPreparation32, false},
		{replay.R21Compatibility, assetdata.LogicalPreparation32, assetdata.LogicalPreparation32Candidate, false},
		{replay.R12Compatibility, assetdata.LogicalPreparation31, assetdata.LogicalPreparation32, true},
		{replay.R12Compatibility, assetdata.LogicalPreparation32Candidate, assetdata.LogicalPreparation32, true},
		{replay.R12Compatibility, "unknown", assetdata.LogicalPreparation32, false},
	} {
		f := &replay.Film{Header: replay.Header{Simulation: tc.profile, Resources: tc.recorded}}
		if got := filmResourceCompatible(f, tc.current); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
