package assets

import (
	"gunship2000-assets/game/world"
	"os"
	"path/filepath"
	"testing"
)

func TestDecoyDerivationLeavesPreparedResourceIdentity(t *testing.T) {
	directory := os.Getenv("GS2000_TEST_MISSIONS")
	if directory == "" {
		t.Skip("prepared front-end assets required")
	}
	identity, err := LogicalResourceProfile(directory)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := LoadWorldAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	if w.Geometries[world.PlayerFlareGeometry] == nil || w.Geometries[world.PlayerChaffGeometry] == nil || w.Combat.Version != 2 {
		t.Fatal("existing cache did not derive source decoys")
	}
	after, err := LogicalResourceProfile(directory)
	if err != nil || after != identity {
		t.Fatal("load changed resource identity", identity, after, err)
	}
	current, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || string(current) != string(manifest) {
		t.Fatal("loader changed frozen preparation", err)
	}
}

func TestDecoyDerivationLoadsRetainedCachesWithoutRewrite(t *testing.T) {
	for _, variable := range []string{"GS2000_TEST_O1_ASSETS", "GS2000_P4_RETAINED28", "GS2000_OLD33_ASSETS"} {
		t.Run(variable, func(t *testing.T) {
			directory := os.Getenv(variable)
			if directory == "" {
				t.Skip("retained prepared cache required")
			}
			before, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			w, err := LoadWorldAssets(directory)
			if err != nil || w.Geometries[world.PlayerFlareGeometry] == nil || w.Geometries[world.PlayerChaffGeometry] == nil {
				t.Fatal("retained raw carrier failed derivation", err)
			}
			after, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
			if err != nil || string(before) != string(after) {
				t.Fatal("retained cache was rewritten", err)
			}
		})
	}
}
