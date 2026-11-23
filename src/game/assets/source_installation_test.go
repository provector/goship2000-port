package assets

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func sourceManifest() *Manifest {
	m := &Manifest{}
	for _, role := range []SourceRole{RoleMainExecutable, rolePictureCatalog, roleScreenCatalog, roleBuilderCatalog, roleBitmapFonts} {
		s := IslandIce469085.Source(role)
		if strings.HasSuffix(s.Name, ".CAT") || s.Name == "GS2000.DAT" {
			raw := make([]byte, 24)
			copy(raw, "TEST.PIC")
			binary.LittleEndian.PutUint32(raw[16:], 3)
			binary.LittleEndian.PutUint32(raw[20:], 26)
			rec := &Entry{Index: 0, DirectoryOffset: 2, RawHex: hex.EncodeToString(raw), NameHex: hex.EncodeToString(raw[:12]), Offset: 26, Length: 3, End: 29}
			m.Resources = append(m.Resources, &Resource{Source: SourceLocation{Path: s.Name, Container: s.Name, Kind: "archive_entry", SHA256: s.SHA256, PayloadSHA256: strings.Repeat("a", 64), Record: rec, Offset: 26, Length: 3}, ExportPath: s.Name + "/TEST.PIC"})
		} else {
			m.Resources = append(m.Resources, &Resource{Source: SourceLocation{Path: s.Name, Kind: "loose_file", SHA256: s.SHA256, PayloadSHA256: s.SHA256, Length: 100}})
		}
	}
	return m
}
func cloneSource(m *Manifest) *Manifest {
	b, _ := json.Marshal(m)
	var q Manifest
	_ = json.Unmarshal(b, &q)
	return &q
}
func TestInstallationLogicalIdentityAndIsolation(t *testing.T) {
	base := sourceManifest()
	for _, prefix := range []string{"", "GS2000CD", "GD/Gunship2000/MPS/GS2000CD"} {
		m := cloneSource(base)
		for _, r := range m.Resources {
			r.Source.Path = path.Join(prefix, r.Source.Path)
			if r.Source.Container != "" {
				r.Source.Container = path.Join(prefix, r.Source.Container)
			}
			r.ExportPath = path.Join(prefix, r.ExportPath)
		}
		c, e := ResolveSourceInstallation(m)
		if e != nil {
			t.Fatal(prefix, e)
		}
		r, e := c.Archive("GS2000.CAT", "TEST.PIC")
		if e != nil || r.ExportPath != path.Join(prefix, "GS2000.CAT/TEST.PIC") {
			t.Fatal("logicalresolution rewrotephysicalprovenance", e, r)
		}
		if _, e = c.Loose("GS.GS2"); e != nil {
			t.Fatal(e)
		}
	}
	cases := map[string]func(*Manifest){
		"mixed-root": func(m *Manifest) { m.Resources[0].Source.Path = "other/GS.GS2" },
		"duplicate-install": func(m *Manifest) {
			q := cloneSource(m)
			for _, r := range q.Resources {
				r.Source.Path = "second/" + r.Source.Path
				if r.Source.Container != "" {
					r.Source.Container = "second/" + r.Source.Container
				}
			}
			m.Resources = append(m.Resources, q.Resources...)
		},
		"filesystem-case": func(m *Manifest) { m.Resources[0].Source.Path = "gs.gs2" },
		"unsupported-hash": func(m *Manifest) {
			m.Resources[0].Source.SHA256 = strings.Repeat("b", 64)
			m.Resources[0].Source.PayloadSHA256 = m.Resources[0].Source.SHA256
		},
		"wrong-container":   func(m *Manifest) { m.Resources[1].Source.Container = "OTHER.CAT" },
		"wrong-extent":      func(m *Manifest) { m.Resources[1].Source.Length++ },
		"bad-raw-name":      func(m *Manifest) { m.Resources[1].Source.Record.NameHex = "00" },
		"truncated-payload": func(m *Manifest) { m.Resources[0].Payload = []byte{1} },
		"missing-core":      func(m *Manifest) { m.Resources = m.Resources[1:] },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := cloneSource(base)
			change(m)
			if _, e := ResolveSourceInstallation(m); e == nil {
				t.Fatal("unsafecontext accepted")
			}
		})
	}
	c, e := ResolveSourceInstallation(base)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Archive("GS2000.CAT", "ABSENT.PIC"); e == nil {
		t.Fatal("missingmember synthesized")
	}
	// Raw12 bytes retain DOS member identity; a case variation is not silently
	// normalized under this explicit supported import policy.
	changed := cloneSource(base)
	rec := changed.Resources[1].Source.Record
	raw, _ := hex.DecodeString(rec.RawHex)
	copy(raw[:12], make([]byte, 12))
	copy(raw, "test.pic")
	rec.RawHex = hex.EncodeToString(raw)
	rec.NameHex = hex.EncodeToString(raw[:12])
	caseContext, e := ResolveSourceInstallation(changed)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = caseContext.Archive("GS2000.CAT", "TEST.PIC"); e == nil || !strings.Contains(e.Error(), "member case") {
		t.Fatal("member case policy missing", e)
	}
	// Duplicate member in one valid container is also ambiguous, even identical.
	m := cloneSource(base)
	m.Resources = append(m.Resources, cloneSource(base).Resources[1])
	c, e = ResolveSourceInstallation(m)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Archive("GS2000.CAT", "TEST.PIC"); e == nil {
		t.Fatal("duplicate member accepted")
	}
}

func TestPreparedRealInstallationContext(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_PREPARED")
	if dir == "" {
		t.Skip("explicit hash-bound prepared fixture required for installed-resource integration")
	}
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	c, e := ResolveSourceInstallation(&m)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range [][2]string{{"GS2000.CAT", "EDITOR.PIC"}, {"MBUILDER.CAT", "MBEDITOR.PIC"}, {"MBUILDER.CAT", "MBICONS.PIC"}, {"GS2000.DAT", "REPLAY.SCN"}} {
		r, e := c.Archive(v[0], v[1])
		if e != nil {
			t.Fatal(v, e)
		}
		if v[1] != "REPLAY.SCN" {
			if _, e = preparedOutput(r.Outputs, "decoded_indices"); e != nil {
				t.Fatal(v, e)
			}
		}
	}
	for _, w := range IslandIce469085.WorldLayout.WorldNames {
		r, e := c.Archive("MBUILDER.CAT", w+"MIS.PIC")
		if e != nil {
			t.Fatal(w, e)
		}
		if _, e = preparedOutput(r.Outputs, "exact_rgb6_components"); e != nil {
			t.Fatal(w, e)
		}
		if w != "PHIL3" {
			if _, e = preparedOutput(r.Outputs, "decoded_indices"); e != nil {
				t.Fatal(w, e)
			}
		}
		if _, e = c.Loose(w + "SMP.GS2"); e != nil {
			t.Fatal(w, e)
		}
	}
}
