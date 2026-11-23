package assets

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownMapAtlasCorrectionAndUnknownControl(t *testing.T) {
	bad, err := os.ReadFile(os.Getenv("GS2000_R221_BAD_ATLAS"))
	if err != nil {
		t.Skip("declared immutable bad/clean atlas comparison required")
	}
	clean, err := os.ReadFile(os.Getenv("GS2000_R221_CLEAN_ATLAS"))
	if err != nil {
		t.Fatal(err)
	}
	if Hash(bad) != Phil1BadAtlasSHA256 || Hash(clean) != Phil1CleanAtlasSHA256 {
		t.Fatal("fixture identity")
	}
	original := append([]byte(nil), bad...)
	got, changed, err := CorrectKnownMapAtlas("PHIL1BMP.GS2", bad)
	if err != nil || !changed || !bytes.Equal(got, clean) || !bytes.Equal(bad, original) {
		t.Fatal("exact correction or input preservation", changed, err)
	}
	if !bytes.Equal(got[:4096], bad[:4096]) || !bytes.Equal(got[12288:], bad[12288:]) {
		t.Fatal("outside corrected block changed")
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{{"PHIL1BMP.GS2", clean}, {"PHIL2BMP.GS2", bad}, {"PHIL1BMP.GS2", bad[:len(bad)-1]}, {"PHIL1BMP.GS2", append([]byte(nil), bad...)}} {
		if len(tc.data) == len(bad) && tc.name == "PHIL1BMP.GS2" && Hash(tc.data) == Phil1BadAtlasSHA256 {
			tc.data[13] ^= 1
		}
		before := append([]byte(nil), tc.data...)
		out, repaired, e := CorrectKnownMapAtlas(tc.name, tc.data)
		if e != nil || repaired || !bytes.Equal(out, before) || !bytes.Equal(tc.data, before) {
			t.Fatal("clean/unfamiliar/wrong-name pass-through", tc.name, e)
		}
	}
}

// Private reader views share immutable payload inodes. Replacing a metadata
// link is atomic copy-on-write and never opens the shared source for writing.
func readerView(t *testing.T, root string) string {
	t.Helper()
	out, e := os.MkdirTemp(filepath.Dir(root), ".r221-reader-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(out) })
	if e = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		r, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		q := filepath.Join(out, r)
		if d.IsDir() {
			return os.MkdirAll(q, 0700)
		}
		return os.Link(p, q)
	}); e != nil {
		t.Fatal(e)
	}
	return out
}
func replacePrivate(t *testing.T, p string, b []byte) {
	t.Helper()
	tmp := p + ".r221-new"
	if e := os.WriteFile(tmp, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(tmp, p); e != nil {
		t.Fatal(e)
	}
}
func currentFixture(t *testing.T) string {
	t.Helper()
	p := os.Getenv("GS2000_TEST_R221_ASSETS")
	if p == "" {
		t.Skip("declared Preparation33 required")
	}
	return p
}

func TestPreparation33Prior32Projection(t *testing.T) {
	root := currentFixture(t)
	view := readerView(t, root)
	raw, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = DecodeObject(raw, &m); e != nil || m.PreparationVersion != 33 {
		t.Fatal(e, m.PreparationVersion)
	}
	raw, e = os.ReadFile(filepath.Join(root, filepath.FromSlash(m.WorldMetadata)))
	if e != nil {
		t.Fatal(e)
	}
	var doc map[string]json.RawMessage
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	var worlds map[string]json.RawMessage
	if e = json.Unmarshal(doc["worlds"], &worlds); e != nil {
		t.Fatal(e)
	}
	var phil map[string]json.RawMessage
	if e = json.Unmarshal(worlds["PHIL1"], &phil); e != nil {
		t.Fatal(e)
	}
	bad, e := os.ReadFile(filepath.Join(root, "world/raw/PHIL1BMP.GS2"))
	if e != nil || Hash(bad) != Phil1BadAtlasSHA256 {
		t.Fatal(e, "raw provenance changed")
	}
	phil["tile_atlas"], _ = json.Marshal(bad)
	worlds["PHIL1"], _ = json.Marshal(phil)
	doc["worlds"], _ = json.Marshal(worlds)
	raw, _ = json.Marshal(doc)
	replacePrivate(t, filepath.Join(view, filepath.FromSlash(m.WorldMetadata)), raw)
	for _, r := range m.Resources {
		for i := range r.Outputs {
			if r.Outputs[i].Path == m.WorldMetadata {
				r.Outputs[i].Size = len(raw)
				r.Outputs[i].SHA256 = Hash(raw)
			}
		}
	}
	m.PreparationVersion = 32
	raw, _ = json.Marshal(m)
	replacePrivate(t, filepath.Join(view, "manifest.json"), raw)
	// Complete prior fingerprint, not equality of selected convenient fields.
	got, e := LogicalResourceProfile(view)
	if e != nil || got != LogicalPreparation32 {
		t.Fatal("full prior32 projection", got, e)
	}
	a, e := LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if Hash(a.World.Worlds["PHIL1"].TileAtlas) != Phil1CleanAtlasSHA256 || a.World.Sources["PHIL1BMP.GS2"] != Phil1BadAtlasSHA256 {
		t.Fatal("decoded atlas/source provenance")
	}
	if e = ValidatePreparation33Capabilities(a); e != nil {
		t.Fatal(e)
	}
	old, e := LoadFrontEndAssets(view)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidatePreparation33Capabilities(old); e == nil {
		t.Fatal("old known-bad cache must request upgrade")
	}
}

func TestPreparation33CompleteDecodedIdentity(t *testing.T) {
	root := currentFixture(t)
	got, e := LogicalResourceProfile(root)
	if e != nil || got != LogicalPreparation33 {
		t.Fatal(got, e)
	}
	r, e := newPreparedReader(root)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := r.read("manifest.json", 64<<20)
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = DecodeObject(raw, &m); e != nil {
		t.Fatal(e)
	}
	b, e := canonicalLogicalResourceDataMode(r, &m, true)
	if e != nil || Hash(b) != decodedPreparation33 {
		t.Fatal("complete decoded profile", Hash(b), e)
	}
}

func TestCleanAtlasCarrierHasSameVerifiedProfile(t *testing.T) {
	root := currentFixture(t)
	view := readerView(t, root)
	clean, e := os.ReadFile(os.Getenv("GS2000_R221_CLEAN_ATLAS"))
	if e != nil || Hash(clean) != Phil1CleanAtlasSHA256 {
		t.Fatal("declared clean counterpart", e)
	}
	raw, e := os.ReadFile(filepath.Join(root, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = DecodeObject(raw, &m); e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile(filepath.Join(root, filepath.FromSlash(m.WorldMetadata)))
	if e != nil {
		t.Fatal(e)
	}
	var doc map[string]json.RawMessage
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	var sources map[string]string
	if e = json.Unmarshal(doc["source_sha256"], &sources); e != nil {
		t.Fatal(e)
	}
	if sources["PHIL1BMP.GS2"] != Phil1BadAtlasSHA256 {
		t.Fatal("expected known raw provenance")
	}
	sources["PHIL1BMP.GS2"] = Phil1CleanAtlasSHA256
	doc["source_sha256"], _ = json.Marshal(sources)
	worldRaw, _ := json.Marshal(doc)
	replacePrivate(t, filepath.Join(view, filepath.FromSlash(m.WorldMetadata)), worldRaw)
	replacements := 0
	for _, r := range m.Resources {
		if r.Source.Kind == "loose_file" && strings.EqualFold(filepath.Base(r.Source.Path), "PHIL1BMP.GS2") {
			if r.Source.SHA256 != Phil1BadAtlasSHA256 || r.Source.PayloadSHA256 != Phil1BadAtlasSHA256 {
				t.Fatal("source fixture")
			}
			r.Source.SHA256, r.Source.PayloadSHA256 = Phil1CleanAtlasSHA256, Phil1CleanAtlasSHA256
		}
		for i := range r.Outputs {
			o := &r.Outputs[i]
			if strings.EqualFold(filepath.Base(o.Path), "PHIL1BMP.GS2") {
				old, e := os.ReadFile(filepath.Join(view, filepath.FromSlash(o.Path)))
				if e != nil || Hash(old) != Phil1BadAtlasSHA256 {
					t.Fatal("raw carrier fixture", e)
				}
				replacePrivate(t, filepath.Join(view, filepath.FromSlash(o.Path)), clean)
				o.Size, o.SHA256 = len(clean), Phil1CleanAtlasSHA256
				replacements++
			}
			if o.Path == m.WorldMetadata {
				o.Size, o.SHA256 = len(worldRaw), Hash(worldRaw)
			}
		}
	}
	if replacements != 2 {
		t.Fatal("expected loose original plus preserved world/raw carrier", replacements)
	}
	raw, _ = json.Marshal(m)
	replacePrivate(t, filepath.Join(view, "manifest.json"), raw)
	got, e := LogicalResourceProfile(view)
	if e != nil || got != LogicalPreparation33 {
		t.Fatal("clean counterpart complete equivalence", got, e)
	}
	a, e := LoadFrontEndAssets(view)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidatePreparation33Capabilities(a); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(a.World.Worlds["PHIL1"].TileAtlas, clean) {
		t.Fatal("clean version was changed")
	}
}
