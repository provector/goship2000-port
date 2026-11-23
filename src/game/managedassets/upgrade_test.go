package managedassets

import (
	"errors"
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingEnemyCapabilityRequestsUpgradeAndPreservesPriorOnFailure(t *testing.T) {
	for _, layout := range []string{"flat", "nested"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			target, original := filepath.Join(root, "assets"), filepath.Join(root, "original")
			if layout == "nested" {
				original = filepath.Join(original, "GS2000CD")
			}
			if err := os.MkdirAll(original, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			m := assets.Manifest{Generator: assets.PreparationGenerator, PreparationVersion: 31, SchemaVersion: assets.PreparedManifestSchema}
			raw, _ := assets.JsonBytes(m)
			if err := os.WriteFile(filepath.Join(target, "manifest.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			prior := []byte("working predecessor payload")
			if err := os.WriteFile(filepath.Join(target, "sentinel"), prior, 0600); err != nil {
				t.Fatal(err)
			}
			prepareCalls := 0
			cause := errors.New("controlled new ENEM extraction failure")
			s := Services{Load: func(string) (*assets.OfficeAssets, error) {
				return &assets.OfficeAssets{Missions: &assets.MissionAssets{Generation: &assets.MissionGenerationData{}}}, nil
			}, Ready: func(a *assets.OfficeAssets) error { return a.Missions.Generation.ValidateEnemyDefinitions() },
				Prepare: func(_, _ string) (*assets.Manifest, error) { prepareCalls++; return nil, cause }, Rename: os.Rename}
			p := configuration.Paths{Directory: root, Original: original, Assets: target, AssetsOrigin: "default-managed"}
			if _, err := Ensure(p, false, io.Discard, s); !errors.Is(err, cause) || prepareCalls != 1 {
				t.Fatal("missing capability did not request one bounded upgrade", prepareCalls, err)
			}
			for name, expected := range map[string][]byte{"manifest.json": raw, "sentinel": prior} {
				b, err := os.ReadFile(filepath.Join(target, name))
				if err != nil || string(b) != string(expected) {
					t.Fatal("previous cache changed", name, err)
				}
			}
		})
	}
}
