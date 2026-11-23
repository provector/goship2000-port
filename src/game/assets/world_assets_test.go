package assets

import (
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func TestWorldProductionPreparationAndPreparedOnlyLoad(t *testing.T) {
	input, e := filepath.Abs(testfixture.Path("installation"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(input, "OBJFILE")); e != nil {
		t.Skip("optional sourceinstallation unavailable")
	}
	output := filepath.Join(t.TempDir(), "prepared")
	m, e := PrepareAssetsWithAudio(input, output, nil)
	if e != nil {
		t.Fatal(e)
	}
	if m.PreparationVersion != PreparationVersion || m.WorldMetadata == "" || m.CockpitMetadata == "" {
		t.Fatal("missingworldreadiness", m.WorldMetadata, m.CockpitMetadata)
	}
	// Loader receives only the prepared tree, and does not consult input paths.
	w, e := LoadWorldAssets(output)
	if e != nil {
		t.Fatal(e)
	}
	if len(w.Models) != 230 || len(w.Worlds) != 14 {
		t.Fatal("coverage")
	}
	r, manifest, e := worldPreparedReader(output)
	if e != nil {
		t.Fatal(e)
	}
	cp, e := loadCockpitAssets(r, manifest)
	if e != nil {
		t.Fatal(e)
	}
	if len(cp.Images) != 21 {
		t.Fatal("cockpitpictures", len(cp.Images))
	}
	outPath := filepath.Join(output, "world/models/001.glb")
	original, e := os.ReadFile(outPath)
	if e != nil {
		t.Fatal(e)
	}
	original[len(original)-1] ^= 1
	if e = os.WriteFile(outPath, original, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadWorldAssets(output); e == nil {
		t.Fatal("modifiedGLBaccepted")
	}
}
func TestUnknownWorldProfileDoesNotUseOffsets(t *testing.T) {
	r := &Resource{Source: SourceLocation{Path: "GS2.GS2", Kind: "loose_file", PayloadSHA256: Hash([]byte("unknown"))}, Payload: []byte("unknown")}
	m := &Manifest{Resources: []*Resource{r}}
	w := &OutputWriter{Root: t.TempDir()}
	if e := exportWorldAssets(w, &pathAllocator{}, m); e != nil {
		t.Fatal(e)
	}
	if m.WorldMetadata != "" || m.CockpitMetadata != "" || len(m.UnsupportedVersionOperations) != 1 {
		t.Fatal("unknownprofilegrantedmetadata")
	}
}
