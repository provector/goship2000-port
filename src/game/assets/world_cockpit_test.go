package assets

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestF5DecoderShortCases(t *testing.T) {
	for _, c := range []struct{ raw, want string }{{"f541840404", "41424142"}, {"f5410202", "414141"}, {"f5410002", "4141"}, {"f590040400", "900201"}, {"f5ff01", "ff"}} {
		raw, _ := hex.DecodeString(c.raw)
		want, _ := hex.DecodeString(c.want)
		got, e := DecodeF5(raw, len(want))
		if e != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %x %v", c.raw, got, e)
		}
	}
	if _, e := DecodeF5([]byte{0xf5, 0}, 1); e == nil {
		t.Fatal("missing consumed codebits accepted")
	}
	if _, e := DecodeF5([]byte{0xf5, 0, 1}, 1); e == nil {
		t.Fatal("initial nextslot garbage accepted")
	}
}

func TestCockpitSourcePreparationAndIndependentReferences(t *testing.T) {
	root := filepath.Clean(testfixture.Path("installation"))
	if _, e := os.Stat(filepath.Join(root, "GS2.GS2")); e != nil {
		t.Skip("optional original source unavailable")
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
	m.PreparationVersion = 21 // Includes source GPS damage carriers and existing HQ/hold literals.
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
	if len(a.Images) != 21 || len(a.Sprites) != 22 || len(a.MapVectors) != 25 {
		t.Fatal("incomplete staticcarriers", len(a.Images), len(a.Sprites), len(a.MapVectors))
	}
	// Independent original coordinate: selected VGA section7 begins3e550,
	// alternative bank13e1 is NOT the temporary GPS world bank10e1.
	gs, e := os.ReadFile(filepath.Join(root, "GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	checkN1PreparedCockpitLiterals(t, gs, a.Literals)
	// Source4:2144 is two literal98h polylines, not the HUD waypoint bitmap.
	// Its source15e0 caller uses shift7, so these signed-byte endpoints draw±4.
	cursorRaw, _ := hex.DecodeString("9801fe000200980100fe000200")
	if !bytes.Equal(gs[0x24290+0x2144:0x24290+0x2144+13], cursorRaw) {
		t.Fatal("GPS cursor original bytes changed")
	}
	cursor := a.MapVectors["gps-fly-cursor"]
	if len(cursor) != 2 || cursor[0].Colour != 0x98 || cursor[1].Colour != 0x98 || cursor[0].Points[0] != [2]int16{-2, 0} || cursor[0].Points[1] != [2]int16{2, 0} || cursor[1].Points[0] != [2]int16{0, -2} || cursor[1].Points[1] != [2]int16{0, 2} {
		t.Fatal("GPS cursor vector differs from independently framed source bytes")
	}
	supportRaw, _ := hex.DecodeString("9804fe000002020000fefe00980100fe00029801fe00020000")
	if !bytes.Equal(gs[0x24290+0x212b:0x24290+0x212b+25], supportRaw) {
		t.Fatal("source Support cursor differs")
	}
	support := a.MapVectors["gps-support-cursor"]
	if len(support) != 3 || len(support[0].Points) != 5 || len(support[1].Points) != 2 || len(support[2].Points) != 2 || support[0].Points[0] != [2]int16{-2, 0} || support[0].Points[4] != [2]int16{-2, 0} {
		t.Fatal("prepared Support cursor source geometry differs")
	}
	box := a.Sprites["gps-overview-box"]
	if box.Width != 11 || box.Height != 9 || !bytes.Equal(box.Pixels, gs[0x24290+0x21bb+4:0x24290+0x21bb+4+99]) {
		t.Fatal("GPS overview viewport marker differs from source4:21bb")
	}
	if !bytes.Equal(a.NightPalette, gs[0x3e550+0x13e1:0x3e550+0x13e1+768]) {
		t.Fatal("prepared night bank differs from original13e1")
	}
	// These are qualified independently source-derived Step31 references, not
	// photographs or a claim of original-frame equivalence.
	for family, want := range map[string]int{"APA": 97, "BLK": 81, "COB": 92, "COM": 94, "DEF": 95, "KIA": 101} {
		art := a.Images[family+"_CPIT.PIC"]
		if art.Bottom != want || art.FullMask[63999] != 0 {
			t.Fatal(family, "mask boundary", art.Bottom)
		}
		if art.Pixels[art.Split-1] != 13 {
			t.Fatal("mask delimiter changed")
		}
		for i, v := range art.FullMask {
			if v != 0 && art.Pixels[i] == 5 {
				t.Fatal("transparent05copied")
			}
		}
	}
	if got := Hash(a.Images["APA_CPIT.PIC"].Pixels); got != "85b9cc906e8326a687c11d84e28b0d862780061c1d65794b567fc637e0a4d8fa" {
		t.Fatal("APAdecode source-reference mismatch", got)
	}
	for i, want := range map[int][2]int16{0: {0, -8}, 32: {10, 0}, 64: {0, 8}, 96: {-10, 0}} {
		if a.Needles[i] != want {
			t.Fatal("handcardinal", i, a.Needles[i])
		}
	}
	metaRaw, e := os.ReadFile(filepath.Join(out.Root, filepath.FromSlash(m.CockpitMetadata)))
	if e != nil {
		t.Fatal(e)
	}
	var doc cockpitMetadata
	if e = json.Unmarshal(metaRaw, &doc); e != nil {
		t.Fatal(e)
	}
	if _, ok := doc.Files["map-palette"]; ok {
		t.Fatal("false universal GPS palette association retained")
	}
	if doc.Files["night-palette"].Kind != "cockpit_night_palette" {
		t.Fatal("night bank semantic metadata missing")
	}
	p := doc.Files["png/APA_CPIT.PIC"]
	f, e := os.Open(filepath.Join(out.Root, filepath.FromSlash(p.Path)))
	if e != nil {
		t.Fatal(e)
	}
	img, e := png.Decode(f)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	gray, ok := img.(*image.Gray)
	if !ok || !bytes.Equal(gray.Pix, a.Images["APA_CPIT.PIC"].Pixels) {
		t.Fatal("gray8transport lost originalindices")
	}
	// A changed indexed payload must fail through the same manifest-bound loader.
	p = doc.Files["image/APA_CPIT.PIC"]
	if e = os.WriteFile(filepath.Join(out.Root, filepath.FromSlash(p.Path)), make([]byte, 64000), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = loadCockpitAssets(reader, *m); e == nil {
		t.Fatal("changedpreparedindices accepted")
	}
}

func TestSettingsHelpSourceTables(t *testing.T) {
	b, e := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if e != nil {
		t.Skip("optional originalsource unavailable")
	}
	help, e := prepareSettingsHelp(b, &IslandIce469085)
	if e != nil {
		t.Fatal(e)
	}
	if len(help) != 9 || help[0].Heading != "COPILOT/GUNNER CONTROLS:" || help[0].Choices[2] != "NONE" || help[1].Choices[3] != "ELITE" || help[7].Body != "" || help[8].Heading != "APU:" || len(help[8].Choices) != 3 || help[8].Choices[2] != "POWER ON" {
		t.Fatal("source help branches changed", help)
	}
}
