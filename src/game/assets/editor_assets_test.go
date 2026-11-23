package assets

import (
	"bytes"
	"fmt"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/internal/testfixture"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentUICapabilitiesAreNotContentCounts(t *testing.T) {
	if e := ValidateCurrentFrontEndCapabilities(&OfficeAssets{}); e == nil {
		t.Fatal("missing film UI admitted")
	}
	a := &OfficeAssets{Replay: &ReplayAssets{Text: map[string]string{"mission_score": "MISSION SCORE: ", "time_taken": "TIME TAKEN: ", "total_kills": "TOTAL KILLS: ", "demo_title": "GUNSHIP 2000: DEMO MODE", "demo_copyright": "Copyright 1991 MicroProse Software Inc."}}}
	a.Cockpit = &cockpit.Assets{MapVectors: map[string][]cockpit.Vector{"gps-support-cursor": make([]cockpit.Vector, 3)}}
	a.Cockpit.Literals = map[string]string{"gps-support-aircraft": "- Aircraft", "gps-support-artillery": "- Artillery : 3"}
	if e := ValidateCurrentFrontEndCapabilities(a); e == nil {
		t.Fatal("missing editor UI admitted")
	}
	a.Editor = &EditorAssets{Background: make([]byte, 64000), Panel: make([]byte, 64000), Icons: make([]byte, 64000), Rectangles: make([]EditorRect, 36), Keyboard: make([]EditorBinding, 70), FillLabels: make([]string, 6), DeleteLabels: make([]string, 19)}
	a.Editor.WindVectors = make([][2]int, 16)
	a.Editor.Text = map[string]string{"initial_status": "GUNSHIP 2000: Mission Builder", "object_number": "Object #", "weapons_prefix": "Weapons: ", "not_located": "Not located", "not_designated": "Not designated", "day": "DAY", "poor": "POOR", "excellent": "EXCELLENT"}
	for k, v := range map[string]string{"located_tail": "ocated at ", "goal_coordinate_format": "%02d/%02d", "goal_detail_format": "%S %cat %s", "farp_location_format": "L%S%s", "base_location_format": "%S l%S%s", "deep_location_format": "%S at %s"} {
		a.Editor.Text[k] = v
	}
	a.Editor.FormationTypes = map[string][]byte{}
	for key, n := range map[string]int{"GULF": 144, "EURO": 158, "PHIL": 109, "ANTC": 144} {
		a.Editor.FormationTypes[key] = make([]byte, n*26)
	}
	a.Debrief = &DebriefAssets{Scenes: map[string]DebriefScene{"REPLACE.SCN": {Background: "FIELD.PIC", Palette: "FIELD.PAL", Areas: make([]OfficeArea, 2), Lori: make([]DebriefLori, 1)}}, Images: map[string][]byte{}, Text: map[int]string{}}
	for _, name := range []string{"FIELD.PIC", "MEMO.PIC", "PILOT.PIC"} {
		a.Debrief.Images[name] = make([]byte, 64000)
	}
	for _, ds := range []int{0x5b82, 0x5b88, 0x5b90, 0x5b96, 0x5d71, 0x5f4e} {
		a.Debrief.Text[ds] = "source text"
	}
	for _, name := range []string{"HAPPY.PIC", "HAPPY_2.PIC", "HAPPY_3.PIC", "SAD.PIC", "SAD_2.PIC", "SAD_3.PIC"} {
		a.Debrief.Images[name] = make([]byte, 64000)
	}
	a.Debrief.Palettes = map[string]color.Palette{}
	for _, name := range []string{"HAPPY.PAL", "SAD.PAL", "SAD_2.PAL", "SAD_3.PAL"} {
		a.Debrief.Palettes[name] = make(color.Palette, 256)
	}
	a.Missions = &MissionAssets{Fonts: map[int]*BitmapFont{2: {}}, Generation: &MissionGenerationData{EnemyDefinitions: sourceEnemyBanks(t)}}
	a.Menus = &MenuAssets{Fonts: map[int]*BitmapFont{3: {}}}
	// Typed capability fixture only: source constructor/PCM production is
	// independently checked by the P7 byte and real preparation tests.
	a.Audio = &AudioAssets{GS2Commands: map[uint16]AudioGS2Command{}, Clips: map[string]*AudioClip{}, ASoundGS2Data: make([]byte, 0x1000)}
	for _, q := range []struct {
		id                  uint16
		row, start, handler int
		raw                 []byte
	}{
		{0x0a, 2, 0x924, 0x93a, []byte{0x8d, 0x0e, 0x24, 0x09, 0xe9, 0xcf, 0xfd}},
		{0x29, 1, 0x8e0, 0x910, []byte{0x8d, 0x0e, 0xe0, 0x08, 0xe9, 0xf2, 0xfd}},
		{0x06, 3, 0x8b4, 0x902, []byte{0x8d, 0x0e, 0xb4, 0x08, 0xe9, 0x0e, 0xfe}},
		{0x2a, 0, 0x99a, 0x933, []byte{0x8d, 0x0e, 0x9a, 0x09, 0xe9, 0xc8, 0xfd}},
	} {
		key := fmt.Sprintf("gs2_%02x", q.id)
		a.Audio.GS2Commands[q.id] = AudioGS2Command{EventID: q.id, Channel: q.row, Start: q.start, HandlerCS: q.handler, HandlerFile: q.handler + 512, HandlerHex: fmt.Sprintf("%x", q.raw), HandlerSHA256: Hash(q.raw), TrackKey: key}
		a.Audio.Clips[key] = &AudioClip{SampleRate: AudioSampleRate, Samples: []int16{1}}
	}
	for k, v := range map[string]string{"delete_title": "DELETE DEFENDERS", "delete1": "Are you absolutely certain", "delete2": "you wish to completely remove"} {
		a.Editor.Text[k] = v
	}

	// There is no film/mission store in this API: empty lists cannot fail readiness.
	if e := ValidateCurrentFrontEndCapabilities(a); e != nil {
		t.Fatal(e)
	}
}
func TestEditorInclusiveBordersAndHistoricalReader(t *testing.T) {
	r := EditorRect{X: 5, Y: 157, Width: 23, Height: 17}
	if !r.Contains(5, 157) || !r.Contains(28, 174) || r.Contains(29, 174) || r.Contains(28, 175) {
		t.Fatal("editor inclusive boundary changed")
	}
	if a, e := loadEditorAssets(nil, Manifest{PreparationVersion: 25}, nil); a != nil || e != nil {
		t.Fatal("historical preparation reader changed", a, e)
	}
}
func TestOriginalEditorResourceOperands(t *testing.T) {
	dir := testfixture.Path("installation")
	var resources []*Resource
	read := func(name string) []byte {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if os.IsNotExist(e) {
			t.Skip("optional original installation unavailable")
		}
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	for _, name := range []string{"GS2000.DAT", "GS2000.CAT", "MBUILDER.CAT"} {
		b := read(name)
		entries, e := parseDirectory(b)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range entries {
			raw := b[v.Offset:v.End]
			r := &Resource{name: v.NameReadable, Payload: raw, backing: b, ExportPath: name + "/" + v.NameReadable, Source: SourceLocation{Path: name, Kind: "archive_entry", Container: name, SHA256: Hash(b), Record: &v, Offset: v.Offset, Length: v.Length, PayloadSHA256: Hash(raw)}}
			if v.NameReadable == "EDITOR.PIC" || v.NameReadable == "MBEDITOR.PIC" || v.NameReadable == "MBICONS.PIC" || len(v.NameReadable) > 7 && v.NameReadable[len(v.NameReadable)-7:] == "MIS.PIC" {
				pix, er := replayPicturePixels(r)
				if er == nil {
					r.Outputs = append(r.Outputs, Output{Path: r.ExportPath + ".indices", Kind: "decoded_indices", Size: len(pix), SHA256: Hash(pix)})
				}
				pal, _, _, er := pictureHeader(raw)
				if er == nil && len(pal) > 0 {
					r.Outputs = append(r.Outputs, Output{Path: r.ExportPath + ".rgb6", Kind: "exact_rgb6_components", Size: 768, SHA256: Hash(pal[0].RGB6)})
				}
			}
			resources = append(resources, r)
		}
	}
	for _, name := range []string{"GULFFORM.DAT", "EUROFORM.DAT", "PHILFORM.DAT", "ANTCFORM.DAT", "OBJDBASE.GS2", "GULFWEAP.DAT", "EUROWEAP.DAT", "PHILWEAP.DAT", "ANTCWEAP.DAT", "FONTS.GS2", "GS.GS2", "EURO6SMP.GS2", "EURO7SMP.GS2", "EURO8SMP.GS2", "EURO9SMP.GS2", "GULF2SMP.GS2", "GULF3SMP.GS2", "GULF4SMP.GS2", "GULF5SMP.GS2", "ANTC1SMP.GS2", "ANTC2SMP.GS2", "ANTC3SMP.GS2", "PHIL1SMP.GS2", "PHIL2SMP.GS2", "PHIL3SMP.GS2"} {
		b := read(name)
		resources = append(resources, &Resource{name: name, Payload: b, Source: SourceLocation{Path: name, Kind: "loose_file", Length: uint64(len(b)), SHA256: Hash(b), PayloadSHA256: Hash(b)}, Outputs: []Output{{Path: "loose/" + name, Kind: "original_payload", Size: len(b), SHA256: Hash(b)}}})
	}
	d, _, e := prepareEditorMetadata(&Manifest{Resources: resources}, &IslandIce469085)
	if e != nil {
		t.Fatal(e)
	}
	if d.Rectangles[1] != (EditorRect{Group: "category_and_view", Index: 1, X: 5, Y: 157, Width: 23, Height: 17}) {
		t.Fatal(d.Rectangles[1])
	}
	if d.BrowserLabels[15] != "BEGIN" || d.BrowserLabels[16] != "RENAME" || d.BrowserLabels[17] != "DELETE" || d.BrowserLabels[19] != "EDIT" || d.BrowserLabels[21] != "NEW" {
		t.Fatal(d.BrowserLabels)
	}
	// Source AREA19width72 becomes35, second x258. No modern equal half72/2 boxes.
	for k, want := range map[int][4]int{19: {222, 177, 35, 10}, 21: {258, 177, 35, 10}} {
		a, ok := d.Browser.Area(k)
		if !ok || a.Rectangle != want {
			t.Fatal(k, a)
		}
	}
	if d.Text["valid"] != "Excellent! you may now save this mission..." || d.Text["browser_title"] != "SIMULATED MISSIONS" {
		t.Fatal(d.Text)
	}
	if d.Previews[0].Width != 89 || d.Previews[0].Height != 65 || d.Previews[0].X != 10 || d.Previews[0].Y != 20 {
		t.Fatal(d.Previews[0])
	}
	again, _, e := prepareEditorMetadata(&Manifest{Resources: resources}, &IslandIce469085)
	if e != nil {
		t.Fatal(e)
	}
	first, _ := JsonBytes(d)
	second, _ := JsonBytes(again)
	if !bytes.Equal(first, second) {
		t.Fatal("editor metadata order is not reproducible")
	}
	if d.Keyboard[1].Segment != 0x1abf || d.Keyboard[1].Offset != 0x368 {
		t.Fatal(d.Keyboard[1])
	}
	if d.ExitRects["valid_save"] != ([4]int{84, 134, 48, 12}) || d.ExitRects["valid_editor"] != ([4]int{188, 134, 48, 12}) || d.ExitRects["valid_abort"] != ([4]int{136, 134, 48, 12}) {
		t.Fatal("exitstate-first geometry")
	}
	if d.Text["abort3"] != "Press NO! to return and continue editing." {
		t.Fatal("abort confirmation text")
	}
	q := d
	q.Text = map[string]string{}
	for k, v := range d.Text {
		q.Text[k] = v
	}
	delete(q.Text, "abort1")
	if validateEditorMetadata(q) == nil {
		t.Fatal("missing current editor confirmation admitted")
	}

	// Verify no source payload mutated during metadata extraction.
	if !bytes.Equal(read("GS.GS2"), resources[len(resources)-15].Payload) {
		t.Fatal("source mutated")
	}
}
func TestPreparedEditorAssetsWhenRequested(t *testing.T) {
	root := os.Getenv("GS2000_TEST_EDITOR")
	if root == "" {
		t.Skip("set explicit fresh prepared bundle")
	}
	a, e := LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidateCurrentFrontEndCapabilities(a); e != nil {
		t.Fatal(e)
	}
	tables := a.Missions.Generation.Tables
	for i, want := range map[int]uint16{0: 631, 1: 72, 9: 1015, 79: 35, 138: 1073, 139: 1131, 151: 688, 158: 87, 229: 530} {
		if tables.EditorModelCosts[i] != want {
			t.Fatalf("original model cost%d got%d want%d", i, tables.EditorModelCosts[i], want)
		}
	}
	for i, want := range []int8{0, 4, 8, 11} {
		if tables.EditorTheatreOffsets[i] != want {
			t.Fatal("source theatre offset", i)
		}
	}
	for i, want := range []uint16{17637, 19631, 19374, 25728, 13040, 16129, 14350, 15883, 14200, 9875, 14554, 18136, 17554, 21847} {
		if tables.EditorWorldBaseCosts[i] != want {
			t.Fatal("source map base cost", i)
		}
	}
	if len(tables.EditorObjectTypes) != 6210 || Hash(tables.EditorObjectTypes) != "a85143130d48e80b24d8a96f99c6583385f7e85c481df30a772417754224e24a" {
		t.Fatal("OBJTYPES profile")
	}
	if a.Editor.Font == nil || len(a.Editor.Previews) != 14 || len(a.Missions.Generation.Tables.EditorModelCosts) != 230 {
		t.Fatal("editor prepared runtime owners missing")
	}
}
