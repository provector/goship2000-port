package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourceInstallationInventoryOriginal(t *testing.T) string {
	t.Helper()
	p := os.Getenv("GS2000_TEST_AUDIO_ORIGINAL")
	if p == "" {
		t.Skip("explicit hash-bound original fixture required for source inventory integration")
	}
	return p
}
func copyFiles(t *testing.T, input, dest string, names []string) {
	t.Helper()
	if e := os.MkdirAll(dest, 0700); e != nil {
		t.Fatal(e)
	}
	for _, name := range names {
		b, e := os.ReadFile(filepath.Join(input, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dest, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestInventoryNestedEditorPreparation(t *testing.T) {
	original := sourceInstallationInventoryOriginal(t)
	entries, e := os.ReadDir(original)
	if e != nil {
		t.Fatal(e)
	}
	names := []string{}
	for _, p := range entries {
		if !p.Type().IsRegular() {
			t.Fatalf("fixture nonregular %s", p.Name())
		}
		names = append(names, p.Name())
	}
	for _, prefix := range []string{"", "GS2000CD"} {
		t.Run(prefix, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "input")
			leaf := root
			if prefix != "" {
				leaf = filepath.Join(root, prefix)
			}
			copyFiles(t, original, leaf, names)
			output := filepath.Join(t.TempDir(), "prepared")
			m, e := PrepareAssetsWithAudio(root, output, nil)
			if e != nil {
				t.Fatal(e)
			}
			if m.EditorMetadata == "" {
				t.Fatal("no current editor metadata")
			}
			// This source-inventory check intentionally omits audio baking. It
			// validates actual editor metadata/readback; central release checks
			// separately call the complete audio/frontend pipeline.
			raw, e := os.ReadFile(filepath.Join(output, filepath.FromSlash(m.EditorMetadata)))
			if e != nil {
				t.Fatal(e)
			}
			var d editorMetadata
			if e = json.Unmarshal(raw, &d); e != nil {
				t.Fatal(e)
			}
			if e = validateEditorMetadata(d); e != nil {
				t.Fatal(e)
			}
			reader, e := newPreparedReader(output)
			if e != nil {
				t.Fatal(e)
			}
			for _, resource := range m.Resources {
				for _, out := range resource.Outputs {
					reader.outputs[out.Path] = out
				}
			}
			for _, o := range d.Images {
				if b, e := reader.verified(o, 64000); e != nil || len(b) != 64000 {
					t.Fatal("editor image readback", e)
				}
			}
			for _, v := range d.Maps {
				if b, e := reader.verified(v.Palette, 768); e != nil || len(b) != 768 {
					t.Fatal("palette readback", e)
				}
				if v.Name == "PHIL3" && v.Unavailable != "" {
					if pixels, err := phil3EditorPixels(reader); err != nil || len(pixels) != 64000 {
						t.Fatal("PHIL3 editor decode", len(pixels), err)
					}
					continue
				}
				if b, e := reader.verified(v.Pixels, 64000); e != nil || len(b) != 64000 {
					t.Fatal("map readback", e)
				}
			}
			for _, v := range d.Previews {
				if b, e := reader.verified(v.Data, 65536); e != nil || len(b) != 4+v.Width*v.Height {
					t.Fatal("preview readback", e)
				}
			}
			for _, r := range m.Resources {
				if r.Source.Kind == "archive_entry" && r.Source.Record.NameReadable == "EDITOR.PIC" {
					want := "GS2000.CAT/EDITOR.PIC"
					if filepath.Clean(m.InputDirectory) != filepath.Clean(leaf) {
						t.Fatal("wrong physical installation", m.InputDirectory)
					}
					if r.ExportPath != want {
						t.Fatal("physicalprovenance rewritten", r.ExportPath)
					}
				}
			}
		})
	}
}
func TestInventorySourceFailuresRetainCause(t *testing.T) {
	original := sourceInstallationInventoryOriginal(t)
	var required []string
	for name := range preparationFiles() {
		required = append(required, name)
	}
	for _, name := range []string{"missing", "conflicting member", "truncated", "unknown-main", "used installation"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			copyFiles(t, original, root, required)
			switch name {
			case "missing":
				if e := os.Remove(filepath.Join(root, "MBUILDER.CAT")); e != nil {
					t.Fatal(e)
				}
			case "truncated":
				if e := os.Truncate(filepath.Join(root, "MBUILDER.CAT"), 30); e != nil {
					t.Fatal(e)
				}
			case "unknown-main":
				if e := os.WriteFile(filepath.Join(root, "GS.GS2"), []byte("unknown executable layout"), 0600); e != nil {
					t.Fatal(e)
				}
			case "conflicting member":
				p := filepath.Join(root, "GS2000.CAT")
				b, e := os.ReadFile(p)
				if e != nil {
					t.Fatal(e)
				}
				copy(b[50:62], b[26:38])
				if e = os.WriteFile(p, b, 0600); e != nil {
					t.Fatal(e)
				}
			case "used installation":
				if e := os.Rename(filepath.Join(root, "GS2000.CAT"), filepath.Join(root, "gs2000.cat")); e != nil {
					t.Fatal(e)
				}
				for _, n := range []string{"SETUP.DAT", "notes.txt", "ROSTER.DAT", "saved.SIM"} {
					if e := os.WriteFile(filepath.Join(root, n), []byte("unrelated"), 0600); e != nil {
						t.Fatal(e)
					}
				}
				copyFiles(t, original, filepath.Join(root, "backup"), []string{"GS.GS2", "GS2000.CAT"})
				f, e := os.Create(filepath.Join(root, "backup.img"))
				if e != nil {
					t.Fatal(e)
				}
				if e = f.Truncate(1 << 30); e != nil {
					t.Fatal(e)
				}
				if e = f.Close(); e != nil {
					t.Fatal(e)
				}
			}
			m, e := PrepareAssetsWithAudio(root, filepath.Join(t.TempDir(), "out"), nil)
			if name == "used installation" {
				if e != nil || m.EditorMetadata == "" {
					t.Fatal("failed to generate editor", e)
				}
				want := preparationFiles()
				for _, r := range m.Resources {
					if _, ok := want[strings.ToUpper(filepath.Base(r.Source.Path))]; !ok {
						t.Fatalf("consumed unrelated %s", r.Source.Path)
					}
				}
				return
			}
			want := map[string]string{"missing": "MBUILDER.CAT", "truncated": "directory", "unknown-main": "executable/table layout", "conflicting member": "conflicting member"}[name]
			if e == nil || !strings.Contains(e.Error(), want) {
				t.Fatalf("expected %s, got %v", want, e)
			}
		})
	}
}

// Audio extraction legitimately uses GS.GS2 without the frontend catalogs. That
// partial scope must not be forced through a complete UI-installation gate.
func TestInventoryKeepsAudioOnlyPartialScope(t *testing.T) {
	original := sourceInstallationInventoryOriginal(t)
	names := []string{"GS.GS2"}
	for _, role := range AudioSourceRoles {
		names = append(names, IslandIce469085.Source(role).Name)
	}
	input := filepath.Join(t.TempDir(), "input")
	copyFiles(t, original, input, names)
	output := filepath.Join(t.TempDir(), "output")
	m, e := prepareAssets(input, output)
	if e != nil {
		t.Fatal("partial raw/audio extraction blocked", e)
	}
	if m.OfficeMetadata != "" || m.EditorMetadata != "" {
		t.Fatal("partial input gained UI capability")
	}
	want, e := os.ReadFile(filepath.Join(input, "ASOUND.GS4"))
	if e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(output, "loose/ASOUND.GS4"))
	if e != nil || string(got) != string(want) {
		t.Fatal("raw audio bytes not retained", e)
	}
}
