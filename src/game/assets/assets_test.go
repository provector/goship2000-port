package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedReaderRejectsTraversalSymlinkAndOversize(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := newPreparedReader(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside", "/absolute", "a/../ok", "a\\b", "C:/escape", ""} {
		if _, err := reader.read(name, 100); err == nil {
			t.Fatalf("unsafe path accepted: %q", name)
		}
	}
	if _, err := reader.read("ok", 3); err == nil {
		t.Fatal("oversized file accepted")
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		if _, err := reader.read("link", 100); err == nil {
			t.Fatal("external symlink accepted")
		}
	}
}

func TestPreparedOutputVerifiesManifestAndContent(t *testing.T) {
	root := t.TempDir()
	data := []byte("data")
	if err := os.WriteFile(filepath.Join(root, "ok"), data, 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := newPreparedReader(root)
	if err != nil {
		t.Fatal(err)
	}
	out := Output{Path: "ok", Kind: "decoded_indices", Size: len(data), SHA256: Hash(data)}
	if _, err := reader.verified(out, 100); err == nil {
		t.Fatal("unindexed output accepted")
	}
	reader.outputs[out.Path] = out
	if _, err := reader.verified(out, 100); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok"), []byte("evil"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.verified(out, 100); err == nil {
		t.Fatal("modified output accepted")
	}
}

func TestOfficeLoaderRejectsMissingMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"schema_version":1,"resources":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOfficeAssets(root); err == nil {
		t.Fatal("manifest without office metadata accepted")
	}
}

// Explicit opt-in keeps normal unit tests independent of proprietary game data.
func TestOfficePreparedAssetsWhenRequested(t *testing.T) {
	directory := os.Getenv("GS2000_TEST_PREPARED")
	if directory == "" {
		t.Skip("set GS2000_TEST_PREPARED for prepared-installation integration")
	}
	assets, err := LoadOfficeAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets.Images) != 5 || len(assets.Palette) != 256 || len(assets.Fonts) != 2 {
		t.Fatal("incomplete office asset set")
	}
	image := make([]byte, 64000)
	if err := assets.Fonts[5].DrawCentered(image, []byte("DUTY ROSTER"), 193, 7, 0, 0); err != nil {
		t.Fatal(err)
	}
	t.Logf("font5 DUTY ROSTER clipped full-scene mask SHA-256: %s", Hash(image))
}

func TestOfficeLoaderRejectsNullResource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"schema_version":1,"office_metadata":"office.json","resources":[null]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOfficeAssets(root); err == nil || !strings.Contains(err.Error(), "null resource") {
		t.Fatalf("expected useful null-resource error, got %v", err)
	}
}

func syntheticOfficeMetadata() OfficeMetadata {
	m := OfficeMetadata{Schema: 1, OuterType: "FORM/SCRN", Initialization: json.RawMessage(`[]`), Rules: json.RawMessage(`{}`)}
	for i := 0; i < 105; i++ {
		group := i
		if group > 31 {
			group = 31
		}
		m.Components = append(m.Components, officeComponent{Index: i, GroupKey: group, SourceAsset: "BASE_AN.PIC", Source: [4]int{0, 0, 1, 1}})
	}
	for key := 0; key < 32; key++ {
		g := OfficeGroup{Key: key}
		for i, c := range m.Components {
			if c.GroupKey == key {
				g.Records = append(g.Records, i)
			}
		}
		m.Groups = append(m.Groups, g)
	}
	for i := 0; i < 10; i++ {
		m.Areas = append(m.Areas, OfficeArea{Key: i, Rectangle: [4]int{0, 0, 1, 1}})
	}
	for i := 0; i < 3; i++ {
		m.Lori = append(m.Lori, officeLori{Key: i, Mode: 1, SourceAsset: "BASE_AN.PIC", Cell: [2]int{1, 1}, Divisor: 1})
	}
	for i := 0; i < 5; i++ {
		a := OfficeAnimation{Key: i, Size: [2]int{15, 9}, Destination: [2]int{136, 85}, FrameCount: 2, HoldCount: 1}
		if i == 1 || i == 2 {
			a.Divisor = 20
			a.Asset = "BASE_AN.PIC"
			for f := 0; f < 2; f++ {
				a.Frames = append(a.Frames, OfficeFrame{Index: f, Source: [4]int{0, f * 9, 15, 9}, Destination: a.Destination})
			}
		}
		if i == 4 {
			a.Asset = "BASE_AN2.PIC"
			a.CallbackSources = map[string][4]int{"even": {0, 191, 15, 9}, "odd": {0, 182, 15, 9}}
		}
		m.Animations = append(m.Animations, a)
	}
	for i := 0; i < 12; i++ {
		m.Ranks = append(m.Ranks, OfficeTextSource{Selector: i})
	}
	for i := 0; i < 5; i++ {
		m.Missions = append(m.Missions, OfficeTextSource{Selector: i})
	}
	for _, key := range []int{0, 1, 2, 3, 4, 5, 8, 9, 10} {
		m.Captions = append(m.Captions, OfficeTextSource{Selector: key})
	}
	m.Denials = make([]OfficeTextSource, 3)
	for i := 0; i < 4; i++ {
		m.Theatres = append(m.Theatres, OfficeTheatre{Selector: i, Segments: make([][4]int, 3)})
	}
	return m
}

func TestOfficeMetadataRejectsUnsafePresentationState(t *testing.T) {
	baseline := syntheticOfficeMetadata()
	if err := validateOfficeMetadata(baseline); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*OfficeMetadata)
	}{
		{"duplicate group key", func(m *OfficeMetadata) { m.Groups[1].Key = 0 }},
		{"unknown group reference", func(m *OfficeMetadata) { m.Groups[0].Records[0] = 999 }},
		{"missing denial captions", func(m *OfficeMetadata) { m.Denials = nil }},
		{"negative animation frame", func(m *OfficeMetadata) { m.Animations[1].Frame = -2 }},
		{"negative animation hold", func(m *OfficeMetadata) { m.Animations[1].Hold = -1 }},
		{"callback out of bounds", func(m *OfficeMetadata) { m.Animations[4].CallbackSources["odd"] = [4]int{0, 199, 15, 9} }},
		{"missing direct frames", func(m *OfficeMetadata) { m.Animations[1].Frames = nil }},
		{"huge rectangle overflow", func(m *OfficeMetadata) { m.Components[0].Source = [4]int{int(^uint(0) >> 1), 0, 2, 1} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(baseline)
			if err != nil {
				t.Fatal(err)
			}
			var m OfficeMetadata
			if err = json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			tc.change(&m)
			if err := validateOfficeMetadata(m); err == nil {
				t.Fatal("unsafe metadata accepted")
			}
		})
	}
}
