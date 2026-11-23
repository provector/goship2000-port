package assets

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticMenuScreen(tag string, bodies ...[]byte) *Resource {
	raw := []byte("FORM\x00\x00\x00\x00SCRN")
	header := make([]byte, 36)
	copy(header, "ROSTER")
	copy(header[9:], "PATCHES")
	add := func(tag string, body []byte) {
		raw = append(raw, []byte(tag)...)
		raw = binary.BigEndian.AppendUint32(raw, uint32(len(body)))
		raw = append(raw, body...)
	}
	add("SCHD", header)
	for _, b := range bodies {
		add(tag, b)
	}
	binary.BigEndian.PutUint32(raw[4:], uint32(len(raw)-8))
	return &Resource{name: "ROSTER.SCN", Payload: raw}
}

func TestMenuScreenBoundedAreaAndLori(t *testing.T) {
	area := []byte{2, 0, 23, 0, 85, 0, 124, 0, 34, 0, 1}
	s, err := parseMenuScreen(syntheticMenuScreen("AREA", area))
	if err != nil {
		t.Fatal(err)
	}
	if s.Background != "ROSTER.PIC" || s.Basenames[1] != "PATCHES" || s.Areas[0].Key != 2 || s.Areas[0].Rectangle != [4]int{23, 85, 124, 34} || s.Areas[0].VisualFlag != 1 {
		t.Fatalf("wrong parsed menu: %+v", s)
	}
	// Nine-byte records really omit trailing bytes; the documented zero-fill
	// reconstructs height34 without reading into the next chunk.
	s, err = parseMenuScreen(syntheticMenuScreen("AREA", area[:9]))
	if err != nil || s.Areas[0].Rectangle[3] != 34 || s.Areas[0].VisualFlag != 0 {
		t.Fatalf("short area: %+v %v", s, err)
	}
	broken := syntheticMenuScreen("AREA", area)
	broken.Payload = broken.Payload[:len(broken.Payload)-1]
	if _, err = parseMenuScreen(broken); err == nil {
		t.Fatal("truncated menu chunk accepted")
	}
	if _, err = parseMenuScreen(syntheticMenuScreen("AREA", []byte{1})); err == nil {
		t.Fatal("short AREA accepted")
	}
	area[3] = 127
	if _, err = parseMenuScreen(syntheticMenuScreen("AREA", area)); err == nil {
		t.Fatal("out of image AREA accepted")
	}
}

func TestMenuEmblemVerticalPackingAndBounds(t *testing.T) {
	lori := []officeLori{{Key: 0, Cell: [2]int{47, 39}, Divisor: 5}, {Key: 4, Base: [2]int{288, 0}, Cell: [2]int{16, 16}, Divisor: 10}}
	emblems, err := menuEmblems(lori)
	if err != nil || len(emblems) != 18 {
		t.Fatalf("emblems: %v", err)
	}
	if emblems[1].Source != [4]int{0, 39, 47, 39} || emblems[17].Source != [4]int{141, 78, 47, 39} || emblems[17].Small != [4]int{304, 112, 16, 16} {
		t.Fatal("emblem selector is not column-major")
	}
	lori[0].Divisor = 0
	if _, err = menuEmblems(lori); err == nil {
		t.Fatal("zero divisor accepted")
	}
	lori[0].Divisor, lori[0].Base = 5, [2]int{300, 0}
	if _, err = menuEmblems(lori); err == nil {
		t.Fatal("out of image emblem accepted")
	}
}

func TestMenuPaletteEvidenceRemainsScoped(t *testing.T) {
	for _, name := range []string{"ROSTER.PIC", "PATCHES.PIC", "MEMO.PIC", "THEATER.PIC"} {
		association, ok := menuPaletteAssociation(name, "GS2000.CAT/BASE.PAL")
		if !ok || association.Diagnostic || association.SHA256 != officePaletteHash {
			t.Fatalf("missing source palette for %s", name)
		}
		if (name == "THEATER.PIC") != (association.Confidence == "static_palette_inheritance") {
			t.Fatal("theatre static prediction conflated with runtime DAC evidence")
		}
	}
	if _, ok := menuPaletteAssociation("FIELD.PIC", "GS2000.CAT/BASE.PAL"); ok {
		t.Fatal("unrelated picture acquired office palette")
	}
}

func TestMenuLoaderRequiresUpgradeWhileOfficeRemainsCompatible(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"schema_version":1,"preparation_version":1,"resources":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMenuAssets(root); err == nil || !strings.Contains(err.Error(), "rebuild/upgrade") {
		t.Fatalf("expected actionable upgrade diagnostic, got %v", err)
	}
	for _, version := range []int{0, 1, 2} {
		if !compatiblePreparation(Manifest{SchemaVersion: 1, PreparationVersion: version}) {
			t.Fatalf("office loader broke older preparation version%d", version)
		}
	}
	if compatiblePreparation(Manifest{SchemaVersion: 1, PreparationVersion: PreparationVersion + 1}) {
		t.Fatal("future preparation version accepted")
	}
}

func TestMenuPreparedAssetsWhenRequested(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MENUS")
	if root == "" {
		t.Skip("set GS2000_TEST_MENUS for source-backed menu integration")
	}
	a, err := LoadMenuAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Images) != 4 || len(a.Emblems) != 18 || len(a.Theatres) != 4 || len(a.Fonts) != 3 || len(a.FullRanks) != 12 || a.FullRanks[7] != "Captain" || a.Strings["mission_prompt"] != "Select Flight Type." {
		t.Fatalf("incomplete source-derived menus: %+v", a)
	}
	if got := fmt.Sprint(a.DecorationWidths); got != "[34 9 11 11 11 8 8 11]" {
		t.Fatalf("award spacing no longer matches original DS:1a82: %s", got)
	}
	if a.Theatres[0].Rectangle != [4]int{148, 47, 30, 20} || a.Theatres[2].Name != "ANTARCTICA" || a.Theatres[2].Rectangle != [4]int{84, 146, 44, 16} {
		t.Fatal("theatre chooser THHD mapping mismatch")
	}
	if len(a.Screens["ROSTER.SCN"].Areas) != 8 || len(a.Screens["ROSTER.SCN"].Lori) != 6 || a.Screens["MEMO.SCN"].Background != "MEMO.PIC" {
		t.Fatal("source screen metadata mismatch")
	}
}
