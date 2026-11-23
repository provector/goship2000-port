package assets

import (
	"bytes"
	"encoding/hex"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"

	"gunship2000-assets/game/cockpit"
)

func TestScenePresentationSourceOperandsAndExport(t *testing.T) {
	root := filepath.Clean(testfixture.Path("installation"))
	raw, e := os.ReadFile(filepath.Join(root, "GS2.GS2"))
	if os.IsNotExist(e) {
		t.Skip("optional original source unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	if Hash(raw) != IslandIce469085.Source(RoleSimulationExecutable).SHA256 {
		t.Fatal("unsupported source identity")
	}
	check := func(at int, expected string) {
		t.Helper()
		b, e := hex.DecodeString(expected)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(raw[at:at+len(b)], b) {
			t.Fatalf("source operand mismatch at%06x", at)
		}
	}
	// Raw instructions establish destination-map lookup, not a guessed flat gray.
	check(0x3e550+0x87, "ea340e471d")
	check(0x3e550+0xe66, "bb340d")
	check(0x3e550+0xe6c, "268a052ed702c4aa")
	check(0x600+0x4a7, "6a1e9a5000e007")
	check(0x600+0x4cc, "68e8006a46")
	check(0x600+0x4d1, "8d84a300506a3eb89c00")
	check(0x600+0x500, "6a3268292a6804006a40")
	// Separate source end-ready prompt uses the same layout, its own literal.
	check(0xd8b0+0x22b2, "6a1e9a5000e007")
	check(0xd8b0+0x22bc, "6a0268292a688802")
	check(0xd8b0+0x22d8, "68e8006a46")
	check(0xd8b0+0x230c, "6a3268292a6888026a40")
	// This table selects AltQ/AltE source branches: only the first draws Quit.
	check(0x600+0x46c, "8c046205")
	check(0x600+0x578, "c70604310000")
	// Source0004 and each later block explicitly push style02,Y,X200.
	check(0xb9b0+0x32, "6a0a9a5000e007")
	for _, q := range []struct {
		style, y, x int
		row         byte
	}{{0x54, 0x5b, 0x5d, 8}, {0x13b, 0x142, 0x144, 14}, {0x1d4, 0x1db, 0x1dd, 20}, {0x2d8, 0x2df, 0x2e1, 26}, {0xa6b, 0xa72, 0xa74, 32}, {0xa97, 0xa9e, 0xaa0, 38}, {0xac3, 0xaca, 0xacc, 48}} {
		check(0xb9b0+q.style, "6a02")
		if raw[0xb9b0+q.y] != 0x6a || raw[0xb9b0+q.y+1] != q.row {
			t.Fatal("source init Y")
		}
		check(0xb9b0+q.x, "68c800")
	}
	m := &Manifest{}
	entries, e := os.ReadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		m.Resources = append(m.Resources, &Resource{name: name, Payload: b, Source: SourceLocation{Path: filepath.Join(root, name), Kind: "loose_file", SHA256: Hash(b), PayloadSHA256: Hash(b), Length: uint64(len(b))}})
	}
	out := &OutputWriter{Root: t.TempDir()}
	if e = prepareWorldCockpit(out, &pathAllocator{}, m, &IslandIce469085); e != nil {
		t.Fatal(e)
	}
	reader, e := newPreparedReader(out.Root)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range m.Resources {
		for _, o := range r.Outputs {
			reader.outputs[o.Path] = o
		}
	}
	a, e := loadCockpitAssets(reader, *m)
	if e != nil {
		t.Fatal(e)
	}
	if a.ScenePresentation.Quit.Text != "Quit to DOS (Y/N)" || a.ScenePresentation.End.Text != "CALL IT A DAY (Y/N)" {
		t.Fatal("source prompt wording")
	}
	for i, want := range []string{"Systems initializing...", "Systems verify...", "  Avionics - CHECK.", "  Optics - CHECK.", "  GPS - CHECK.", "  Weapons - CHECK.", " All systems O.K."} {
		if a.ScenePresentation.Initialization[i].Text != want {
			t.Fatal("source literal spacing", i)
		}
	}
	for index, want := range map[int]byte{0: 0, 10: 8, 15: 10, 30: 9, 41: 5, 128: 3, 232: 2, 255: 10} {
		if a.ScenePresentation.ConfirmationRemap[index] != want {
			t.Fatal("independent remap byte", index)
		}
	}
	// Independently summed original width bytes: Quit68,End82. These bounds
	// exercise the real prepared font, not a round-trip through the drawer.
	for _, tc := range []struct {
		end  bool
		want cockpit.Rect
	}{{false, cockpit.Rect{122, 62, 197, 70}}, {true, cockpit.Rect{115, 62, 204, 70}}} {
		got, e := cockpit.SceneConfirmationBounds(a, tc.end)
		if e != nil || got != tc.want {
			t.Fatal("source prompt measured bounds", got, tc.want, e)
		}
	}
	frame := bytes.Repeat([]byte{41}, 64000)
	if e = cockpit.DrawSceneConfirmation(frame, a, false); e != nil {
		t.Fatal(e)
	}
	if frame[62*320+122] != 237 || frame[61*320+122] != 41 {
		t.Fatal("remap bounds or destination preservation")
	}
	if e = cockpit.DrawInitialization(frame, a, 7); e != nil {
		t.Fatal(e)
	}
}
