package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func profileFixture(t *testing.T) string {
	t.Helper()
	p := os.Getenv("GS2000_TEST_EDITOR")
	if p == "" {
		t.Skip("explicit current prepared fixture required")
	}
	return p
}
func TestLogicalResourceProfileFixtures(t *testing.T) {
	paths := os.Getenv("GS2000_P1_PROFILE_FIXTURES")
	if paths == "" {
		paths = profileFixture(t)
	}
	var current string
	for _, p := range strings.Split(paths, string(os.PathListSeparator)) {
		h, e := LogicalResourceProfile(p)
		if e != nil {
			t.Fatal(p, e)
		}
		t.Log(p, h)
		b, e := os.ReadFile(filepath.Join(p, "manifest.json"))
		if e != nil {
			t.Fatal(e)
		}
		var m Manifest
		if e = json.Unmarshal(b, &m); e != nil {
			t.Fatal(e)
		}
		if m.PreparationVersion == 26 {
			if current != "" && current != h {
				t.Fatal("source relocation changed semantic profile")
			}
			current = h
			if h != LogicalPreparation26 {
				t.Fatal("complete current profile changed", h)
			}
		}
	}
}
func copyPrepared(t *testing.T, root string) string {
	t.Helper()
	target := t.TempDir()
	if e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		out := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0700)
		}
		if d.Type()&os.ModeSymlink != 0 {
			t.Fatalf("copy fixture contains symlink %s", p)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(out, b, 0600)
	}); e != nil {
		t.Fatal(e)
	}
	return target
}
func TestLogicalProfileRejectsChangedContent(t *testing.T) {
	target := copyPrepared(t, profileFixture(t))
	mp := filepath.Join(target, "manifest.json")
	original, e := os.ReadFile(mp)
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = json.Unmarshal(original, &m); e != nil {
		t.Fatal(e)
	}
	if _, e = LogicalResourceProfile(target); e != nil {
		t.Fatal(e)
	}
	var item *Resource
	var outIndex int
	for _, r := range m.Resources {
		for i, o := range r.Outputs {
			if o.Kind == "mission_preparation_metadata" {
				item = r
				outIndex = i
			}
		}
	}
	if item == nil {
		t.Fatal("source metadata prerequisite missing")
	}
	out := item.Outputs[outIndex]
	p := filepath.Join(target, filepath.FromSlash(out.Path))
	raw, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	bad := append([]byte(nil), raw...)
	bad[len(bad)/2] ^= 1
	if e = os.WriteFile(p, bad, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = LogicalResourceProfile(target); e == nil || !strings.Contains(e.Error(), "SHA-256") {
		t.Fatal("changed actual bytes escaped validation", e)
	}
	if e = os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	// Declaring a matching new hash cannot grant a new semantic interpretation.
	var v map[string]any
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	v["audit_altered_table"] = "different semantic bytes"
	bad, e = json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, bad, 0600); e != nil {
		t.Fatal(e)
	}
	item.Outputs[outIndex].SHA256 = Hash(bad)
	item.Outputs[outIndex].Size = len(bad)
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(mp, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = LogicalResourceProfile(target); e == nil || !strings.Contains(e.Error(), "unsupported logical resource content") {
		t.Fatal("edited manifest admitted changed table", e)
	}
	// A matching declared digest must not hide another JSON value from readers.
	trailing := append(append([]byte(nil), raw...), []byte("\n{}")...)
	if e = os.WriteFile(p, trailing, 0600); e != nil {
		t.Fatal(e)
	}
	item.Outputs[outIndex].SHA256 = Hash(trailing)
	item.Outputs[outIndex].Size = len(trailing)
	b, _ = json.Marshal(m)
	if e = os.WriteFile(mp, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = LogicalResourceProfile(target); e == nil || !strings.Contains(e.Error(), "exactly one value") {
		t.Fatal("trailingJSON hiddenbycanonicalization", e)
	}
	if e = os.WriteFile(p, bad, 0600); e != nil {
		t.Fatal(e)
	}
	item.Outputs[outIndex].SHA256 = Hash(bad)
	item.Outputs[outIndex].Size = len(bad)
	b, _ = json.Marshal(m)
	if e = os.WriteFile(mp, b, 0600); e != nil {
		t.Fatal(e)
	}
	// Path-looking text is not a file reference: it must remain exact semantic data.
	reader, e := newPreparedReader(target)
	if e != nil {
		t.Fatal(e)
	}
	base, e := canonicalLogicalResourceData(reader, &m)
	if e != nil {
		t.Fatal(e)
	}
	v["audit_altered_table"] = "GS2000.CAT/EDITOR.PIC"
	bad, _ = json.Marshal(v)
	os.WriteFile(p, bad, 0600)
	item.Outputs[outIndex].SHA256 = Hash(bad)
	item.Outputs[outIndex].Size = len(bad)
	other, e := canonicalLogicalResourceData(reader, &m)
	if e != nil {
		t.Fatal(e)
	}
	if Hash(base) == Hash(other) {
		t.Fatal("runtime text was normalized as provenance")
	}
	if e = os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(mp, original, 0600); e != nil {
		t.Fatal(e)
	}
	// Interpretation/provenance fields other than verified location are retained.
	m.Policy = append(m.Policy, "changed interpretation")
	item.Outputs[outIndex] = out
	b, _ = json.Marshal(m)
	os.WriteFile(mp, b, 0600)
	if _, e = LogicalResourceProfile(target); e == nil {
		t.Fatal("changed interpretation admitted")
	}
}

func TestSteamWorkingProvenanceIsNotSimulationState(t *testing.T) {
	flat, nested := os.Getenv("GS2000_P1_FLAT_ASSETS"), os.Getenv("GS2000_P1_STEAM_ASSETS")
	if flat == "" || nested == "" {
		t.Skip("explicit flat/Steam prepared fixtures required")
	}
	a, e := LoadWorldAssets(flat)
	if e != nil {
		t.Fatal(e)
	}
	b, e := LoadWorldAssets(nested)
	if e != nil {
		t.Fatal(e)
	}
	if a.Sources["OBJECT.DAT"] == b.Sources["OBJECT.DAT"] {
		t.Fatal("fixture pair doesnot exercise different original mutable records")
	}
	x, e := LogicalResourceProfile(flat)
	if e != nil {
		t.Fatal(e)
	}
	y, e := LogicalResourceProfile(nested)
	if e != nil {
		t.Fatal(e)
	}
	if x != y || x != LogicalPreparation26 {
		t.Fatal("provenance-only original workfiles changed gameplay profile", x, y)
	}
}
