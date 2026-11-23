package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/replay"
	"testing"
)

func TestUIResourceMappingRetainsStrictKnownProfiles(t *testing.T) {
	for _, r := range []string{assetdata.LogicalPreparation30, assetdata.LogicalPreparation29, assetdata.LogicalPreparation28, assetdata.LogicalPreparation27, assetdata.LogicalPreparation26} {
		f := &replay.Film{Header: replay.Header{Resources: r, Simulation: replay.P5Compatibility}}
		if !filmResourceCompatible(f, assetdata.LogicalPreparation30) {
			t.Fatal("known complete UI-only predecessor rejected", r)
		}
	}
	f := &replay.Film{Header: replay.Header{Resources: "arbitrary-manifest", Simulation: replay.P5Compatibility}}
	if filmResourceCompatible(f, assetdata.LogicalPreparation30) {
		t.Fatal("unknown bundle admitted")
	}
}
