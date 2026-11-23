package assets

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

func replayOriginalMetadata(t *testing.T) replayMetadata {
	t.Helper()
	root := testfixture.Path("installation")
	var resources []*Resource
	read := func(name string) []byte {
		b, e := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(e) {
			t.Skip("optional original profile unavailable")
		}
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	for _, name := range []string{"GS2000.DAT", "GS2000.CAT"} {
		raw := read(name)
		dir, e := parseDirectory(raw)
		if e != nil {
			t.Fatal(e)
		}
		for _, v := range dir {
			b := raw[v.Offset:v.End]
			r := &Resource{name: v.NameReadable, Payload: b, backing: raw, Source: SourceLocation{Path: name, Kind: "archive_entry", SHA256: Hash(raw), Offset: v.Offset, PayloadSHA256: Hash(b)}}
			if r.name == "REPLAY.PIC" {
				pixels, e := replayPicturePixels(r)
				if e != nil {
					t.Fatal(e)
				}
				r.Outputs = []Output{{Path: "catalog/REPLAY.indices", Kind: "decoded_indices", Size: len(pixels), SHA256: Hash(pixels)}}
			}
			resources = append(resources, r)
		}
	}
	for _, name := range []string{"GS.GS2", "GS2.GS2"} {
		b := read(name)
		r := &Resource{name: name, Payload: b, Source: SourceLocation{Path: name, Kind: "loose_file", SHA256: Hash(b), PayloadSHA256: Hash(b)}}
		if name == "GS2.GS2" {
			panel, e := DecodeF5(read("REPLAY.PIC"), 64000)
			if e != nil {
				t.Fatal(e)
			}
			r.Outputs = []Output{{Path: "world/cockpit/images/REPLAY.PIC.indices", Kind: "cockpit_indexed_pixels", Size: 64000, SHA256: Hash(panel)}}
		}
		resources = append(resources, r)
	}
	d, _, e := prepareReplayMetadata(&Manifest{Resources: resources}, &IslandIce469085)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestReplayOriginalResourceAndControlOperands(t *testing.T) {
	d := replayOriginalMetadata(t)
	if d.Text["title"] != "MISSION FILMS" || d.Text["last_mission"] != "Last Mission" || d.Text["original_score"] != "Original score\n" || d.Text["resumed_score"] != "\nResumed score\n" || d.Text["export_unavailable"] != "Cannot EXPORT this mission." {
		t.Fatal(d.Text)
	}
	if d.Text["destroyed_by"] != " destroyed by " || d.Text["rescued_by"] != " rescued by " || d.Text["damage_1"] != "avionics" || d.Text["damage_10"] != "starboard engine" || d.Text["damage_11"] != "no damage" {
		t.Fatal(d.Text)
	}
	if d.BrowserImage.SHA256 != "45540cbd4a1340e4527f152e92d7d4417edc753b5c1391bdc71ef0ad65d4f247" || d.BrowserPalette.SHA256 != officePaletteHash || d.BrowserPalette.Diagnostic {
		t.Fatal("source image/palette association")
	}
	if d.ViewerRegions[0] != (ReplayRegion{33, 172, 52, 181, 0x1372}) || d.ViewerRegions[17] != (ReplayRegion{291, 172, 315, 181, 0x1500}) {
		t.Fatal(d.ViewerRegions)
	}
	if d.Styles.TitleInset != [2]int{6, 4} || d.Styles.TitleFont != 5 || d.Styles.ListClear != 8 || d.Styles.SelectedInk != 15 || d.Styles.DetailClear != 6 || d.Styles.ClockMinutes != [2]int{197, 173} || d.Styles.ClockSeconds != [2]int{209, 173} {
		t.Fatal(d.Styles)
	}
	for key, r := range map[int][4]int{0: {160, 0, 160, 191}, 1: {171, 18, 120, 10}, 12: {171, 128, 120, 10}, 14: {299, 14, 15, 0}, 15: {171, 144, 40, 10}, 19: {222, 177, 72, 10}, 20: {222, 145, 72, 30}} {
		a, ok := d.Browser.Area(key)
		if !ok || a.Rectangle != r {
			t.Fatalf("AREA%d %+v", key, a)
		}
	}
	if d.BrowserLabels[15] != "PLAY" || d.BrowserLabels[16] != "RENAME" || d.BrowserLabels[17] != "DELETE" || d.BrowserLabels[18] != "EXIT" || d.BrowserLabels[19] != "EXPORT" {
		t.Fatal(d.BrowserLabels)
	}
	// Raw source pointer bytes start9f then nine transparent bytes; no invented
	// atlas crop, RGB replacement or host cursor bitmap is used.
	if !bytes.Equal(d.ViewerPointer[:10], []byte{0x9f, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatal("source pointer")
	}
}
func TestReplayStrictBoundsAndExactIndexHighlight(t *testing.T) {
	a := &ReplayAssets{ViewerRegions: []ReplayRegion{{33, 172, 52, 181, 0x1372}}, ViewerPointer: make([]byte, 100)}
	for _, xy := range [][2]int{{33, 175}, {52, 175}, {40, 172}, {40, 181}} {
		if a.RegionHit(xy[0], xy[1]) != -1 {
			t.Fatal("border admitted", xy)
		}
	}
	if a.RegionHit(34, 173) != 0 || a.RegionHit(51, 180) != 0 {
		t.Fatal("interior rejected")
	}
	frame := bytes.Repeat([]byte{12}, 64000)
	frame[174*320+35] = 5
	a.ReplaceRegionIndices(frame, 0, 12, 7)
	if frame[174*320+35] != 5 || frame[174*320+34] != 7 || frame[181*320+34] != 12 || frame[174*320+52] != 12 {
		t.Fatal("highlight changed unrelated palette indices/bounds")
	}
	frame = bytes.Repeat([]byte{9}, 64000)
	a.ViewerPointer[0] = 0x9f
	a.DrawViewerPointer(frame, 20, 30)
	if frame[30*320+20] != 0x9f || frame[30*320+21] != 9 {
		t.Fatal("zero transparency")
	}
}
func TestReplayMetadataReadinessAndHistoricalPreparation(t *testing.T) {
	if _, e := loadReplayAssets(&preparedReader{}, Manifest{PreparationVersion: 23}, nil); e != nil {
		t.Fatal("older loader compatibility", e)
	}
	if _, e := loadReplayAssets(&preparedReader{}, Manifest{PreparationVersion: 24}, nil); e == nil {
		t.Fatal("new incomplete preparation admitted")
	}
	d := replayOriginalMetadata(t)
	d.ViewerRegions[0].X2 = 500
	if e := validateReplayMetadata(d, true); e == nil {
		t.Fatal("bad region admitted")
	}
}
func TestReplayPreparedOnlyWhenRequested(t *testing.T) {
	root := os.Getenv("GS2000_TEST_REPLAY")
	if root == "" {
		t.Skip("set GS2000_TEST_REPLAY to preparation24")
	}
	a, e := LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if a.Replay == nil || len(a.Replay.BrowserImage) != 64000 || len(a.Replay.ViewerPanel) != 64000 || len(a.Replay.ViewerPointer) != 100 || a.Replay.Fonts[3] == nil || a.Replay.Fonts[5] == nil {
		t.Fatal("typed film resource readiness")
	}
	if bytes.Equal(a.Replay.BrowserImage, a.Replay.ViewerPanel) {
		t.Fatal("catalog and loose replay images conflated")
	}
}
