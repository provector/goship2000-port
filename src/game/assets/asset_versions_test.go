package assets

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func profileResourcesForTest() []*Resource {
	p := &IslandIce469085
	var resources []*Resource
	for _, role := range []SourceRole{rolePictureCatalog, roleScreenCatalog, RoleMainExecutable, roleBitmapFonts} {
		s := p.Source(role)
		kind := "loose_file"
		if role == rolePictureCatalog || role == roleScreenCatalog {
			kind = "archive_entry"
		}
		resources = append(resources, &Resource{name: s.Name, Source: SourceLocation{Path: s.Name, Kind: kind, SHA256: s.SHA256, PayloadSHA256: s.SHA256}})
	}
	for _, role := range []SourceRole{roleOfficeScreen, roleOfficePalette} {
		s := p.Source(role)
		parent := p.Source(roleScreenCatalog)
		if role == roleOfficePalette {
			parent = p.Source(rolePictureCatalog)
		}
		resources = append(resources, &Resource{name: s.Name, Source: SourceLocation{Path: parent.Name, Kind: "archive_entry", SHA256: parent.SHA256, PayloadSHA256: s.SHA256}})
	}
	return resources
}

func TestSupportedOfficeVersionRequiresSourceCombination(t *testing.T) {
	resources := profileResourcesForTest()
	p, err := recognizeOfficeProfile(resources)
	if err != nil || p != &IslandIce469085 {
		t.Fatal(p, err)
	}
	for i := range resources {
		original := resources[i].Source
		if i < 4 {
			resources[i].Source.SHA256 = "unknown"
		} else {
			resources[i].Source.PayloadSHA256 = "unknown"
		}
		if _, err := recognizeOfficeProfile(resources); err == nil {
			t.Fatalf("accepted mixed profile after changing source %d", i)
		}
		resources[i].Source = original
	}
	if _, err := recognizeOfficeProfile(resources[:3]); err == nil || !strings.Contains(err.Error(), "FONTS.GS2") {
		t.Fatal("missing source did not identify its prerequisite", err)
	}
}

func TestUnsupportedOfficeInterpretationRetainsExtraction(t *testing.T) {
	m := &Manifest{Resources: []*Resource{{name: "BASE.SCN", Source: SourceLocation{PayloadSHA256: officeScreenHash}}}}
	if err := exportOfficeMetadata(nil, nil, m); err != nil {
		t.Fatal(err)
	}
	if m.OfficeMetadata != "" || len(m.UnsupportedVersionOperations) != 1 || !strings.Contains(m.UnsupportedVersionOperations[0].Reason, "GS.GS2") {
		t.Fatalf("unsupported operation not reported: %+v", m)
	}
	// Unknown BASE content remains raw; no fixed-layout parser is entered.
	m.Resources[0].Source.PayloadSHA256 = "unknown"
	m.UnsupportedVersionOperations = nil
	if err := exportOfficeMetadata(nil, nil, m); err != nil || len(m.UnsupportedVersionOperations) != 1 {
		t.Fatal(m, err)
	}
}

func TestUnknownCatalogIdentityStillExtractsAndDecodes(t *testing.T) {
	picture := codeFixture([]int{1, 2, 3}, nil, 3, 1, 11)
	raw := make([]byte, 26)
	binary.LittleEndian.PutUint16(raw, 1)
	copy(raw[2:], "BASE.PIC")
	binary.LittleEndian.PutUint32(raw[18:], uint32(len(picture)))
	binary.LittleEndian.PutUint32(raw[22:], 26)
	raw = append(raw, picture...)
	in := t.TempDir()
	if err := os.WriteFile(filepath.Join(in, "GS2000.CAT"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "assets")
	m, err := prepareAssets(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Containers) != 1 || m.Containers[0].BaselineIdentity || m.VersionProfile != "" {
		t.Fatalf("unknown catalog received known identity: %+v", m)
	}
	if m.Summary.Entries != 1 || m.Summary.DiagnosticPictures != 1 || m.Summary.ColourPictures != 0 {
		t.Fatalf("generic processing blocked: %+v", m.Summary)
	}
	r := m.Resources[0]
	if r.Conversion.RuntimeValidation != "none; offline conversion does not establish runtime use" || !r.Conversion.Palette.Diagnostic {
		t.Fatal("unknown source borrowed historical validation", r.Conversion)
	}
	for _, o := range r.Outputs {
		if o.Kind == "decoded_indices" {
			indices, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(o.Path)))
			if err != nil || !bytes.Equal(indices, []byte{1, 2, 3}) {
				t.Fatal(indices, err)
			}
		}
	}
	extracted, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(r.ExportPath)))
	if err != nil || !bytes.Equal(extracted, picture) {
		t.Fatal("raw source changed", err)
	}
}

func TestUnknownFontSourceDoesNotReceiveKnownLayout(t *testing.T) {
	source := []byte("not the known font bank")
	r := &Resource{name: "FONTS.GS2", Payload: source, Source: SourceLocation{SHA256: Hash(source)}}
	if supported, err := exportOfficeFonts(nil, r, nil); supported || err != nil {
		t.Fatal(supported, err)
	}
	if IslandIce469085.SourceMatches(roleBitmapFonts, Hash(source)) {
		t.Fatal("unrecognized font accepted")
	}
}
