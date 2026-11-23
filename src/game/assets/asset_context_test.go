package assets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOutputPaletteContextsRemainIndependent(t *testing.T) {
	r := &Resource{ExportPath: "CAT/POINTER.PIC"}
	base := PaletteAssociation{Source: "CAT/BASE.PAL", Evidence: "office context"}
	field := PaletteAssociation{Source: "CAT/FIELD.PAL", Evidence: "mission context"}
	r.Conversion.Palette = base
	w := &OutputWriter{Root: t.TempDir()}
	a := &pathAllocator{}
	if err := addPalettedGenerated(w, r, a, ".png", "indexed_picture_png", []byte("office"), base); err != nil {
		t.Fatal(err)
	}
	if err := addPalettedGenerated(w, r, a, ".mission.png", "mission_supported_colour_png", []byte("briefing"), field); err != nil {
		t.Fatal(err)
	}
	field.Source = "changed local variable"
	if r.Outputs[0].Palette.Source != "CAT/BASE.PAL" || r.Outputs[1].Palette.Source != "CAT/FIELD.PAL" || r.Conversion.Palette.Source != "CAT/BASE.PAL" {
		t.Fatalf("palette contexts lost: %+v", r)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var reopened Resource
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	if reopened.Outputs[0].Palette.Source != "CAT/BASE.PAL" || reopened.Outputs[1].Palette.Source != "CAT/FIELD.PAL" {
		t.Fatal("output context did not survive manifest JSON")
	}
	// Prior manifests intentionally have no per-output context; loaders continue
	// using the established screen-specific metadata instead of rejecting them.
	var old Output
	if err := json.Unmarshal([]byte(`{"path":"old.png","kind":"indexed_picture_png"}`), &old); err != nil || old.Palette != nil {
		t.Fatal(old, err)
	}
}

func TestStartupDetectionDescribesSignaturesNotCodecReadiness(t *testing.T) {
	for _, raw := range [][]byte{[]byte("MADSS1"), {0xff, 0xff, 0x5a, 0x14}} {
		_, basis, executable := identify(raw, "unknown")
		if executable || strings.Contains(basis, "unresolved") || !strings.Contains(basis, "variant specific") {
			t.Fatal(basis)
		}
	}
}
