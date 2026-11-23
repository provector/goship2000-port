package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func briefingTestFont() *assetdata.BitmapFont {
	mask := image.NewPaletted(image.Rect(0, 0, 1, 7), color.Palette{color.Black, color.White})
	mask.Pix[0] = 1
	metrics := assetdata.FontMetrics{SchemaVersion: 1, Height: 7, First: 32, Last: 126, AtlasSize: [2]int{1, 7}, AtlasMaskSHA256: assetdata.Hash(mask.Pix)}
	for i := 32; i < 127; i++ {
		metrics.Glyphs = append(metrics.Glyphs, assetdata.FontGlyph{Byte: i, Rect: [4]int{0, 0, 1, 7}, Width: 1, Advance: 1})
	}
	font, err := assetdata.NewBitmapFont(metrics, mask)
	if err != nil {
		panic(err)
	}
	return font
}

func TestBriefingFormatPreservesSourceCommands(t *testing.T) {
	got := briefingFormat("OPS:%t%c%s%c %02d %%c%d`", "ABC", 3, 180)
	if got != "OPS:%t%cABC%c 03 %c180`" {
		t.Fatal(got)
	}
}

func TestBriefingParagraphToggleTabAndWrap(t *testing.T) {
	font := briefingTestFont()
	v := &BriefingView{Assets: &assetdata.MissionAssets{Fonts: map[int]*assetdata.BitmapFont{2: font}}}
	f := make([]byte, 64000)
	y := v.paragraph(f, "A%t%cB%c\nC", 10, 24, 30, 80, 5, 20)
	if y != 32 || f[24*320+10] != 0 || f[24*320+15] != 4 || f[32*320+10] != 0 {
		t.Fatal("control positioning/colours", y, f[24*320+15])
	}
	y = v.paragraph(f, "AAAA BBBB", 10, 24, 6, 80, 5, 20)
	if y != 32 {
		t.Fatal("word did not wrap", y)
	}
}

func TestCommanderAdvanceUsesSourceShownPathAndSeparateRNG(t *testing.T) {
	img := make([]byte, 64000)
	for i := range img {
		img[i] = byte(i % 251)
	}
	font := briefingTestFont()
	a := &assetdata.MissionAssets{Images: map[string][]byte{"FIELD.PIC": img, "COMMANDR.PIC": img}, Fonts: map[int]*assetdata.BitmapFont{2: font, 3: font}, Metadata: assetdata.MissionPresentationData{Transitions: map[string][]int{"1->2": {1, 2}, "1->3": {4, 5, 6, 7}, "2->1": {3, 2}, "2->3": {9, 10, 11, 12}, "3->1": {8, 7, 6, 5}, "3->2": {13, 12, 11, 10}}}}
	for i := 0; i < 14; i++ {
		a.Metadata.Poses = append(a.Metadata.Poses, assetdata.CommanderPose{Index: i, Body: [4]int{0, 32, 80, 56}, Arms: [4]int{0, 88, 40, 62}})
	}
	m := &gamestate.MissionData{StableScene: "FIELD", ObjectHash: "immutable"}
	p := &gamestate.Pilot{Name: "A"}
	v, err := NewBriefingView(a, m, p, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.commander.Pose != 1 || v.commander.State != 1 {
		t.Fatal("native initial idle pose", v.commander)
	}
	draws := append([]gamestate.MissionDraw(nil), m.Draws...)
	before := append([]byte(nil), v.scene...)
	for i := 0; i < 400; i++ {
		v.Tick()
	}
	if m.ObjectHash != "immutable" || len(m.Draws) != len(draws) {
		t.Fatal("effects mutated generation")
	}
	if bytes.Equal(before, v.scene) {
		t.Fatal("no visual progression")
	}
	v.commander.Path = []int{4, 5, 6, 7}
	v.commander.Index = 0
	v.commander.Repeat = 10
	for _, want := range []int{4, 5, 6, 7, 7} {
		v.advanceCommander(true)
		if v.commander.Pose != want {
			t.Fatalf("pose got%d want%d", v.commander.Pose, want)
		}
	}
}

func TestBriefingMapCoordinatesSignedTruncation(t *testing.T) {
	x, y := gamestate.MissionMapPoint(gamestate.MissionPoint{-3276, -4095}, 0, 0)
	if x != 19 || y != 145 {
		t.Fatal(x, y)
	}
	x, y = gamestate.MissionMapPoint(gamestate.MissionPoint{3277, 4096}, 0, 0)
	if x != 20 || y != 144 {
		t.Fatal(x, y)
	}
	if gamestate.MissionGrid(-0x4001) != 0 {
		t.Fatal("negative signed division rounded down")
	}
}

func TestBriefingVisibilityDrawStableBetweenFrames(t *testing.T) {
	// Visibility is selected when entering the Intel page; frame redraw itself
	// never consumes it. The controller owns page transitions, not mission data.
	font := briefingTestFont()
	a := &assetdata.MissionAssets{Generation: &assetdata.MissionGenerationData{Tables: assetdata.MissionTables{VisibilityPairs: [2][2]string{{"Good", "Excellent"}, {"Poor", "Limited"}}}}, Fonts: map[int]*assetdata.BitmapFont{2: font, 3: font}, Images: map[string][]byte{"FIELD.PIC": make([]byte, 64000)}, Metadata: assetdata.MissionPresentationData{Templates: map[int]string{0x3577: "Visibility is %s."}}}
	v, err := NewBriefingView(a, &gamestate.MissionData{StableScene: "FIELD"}, &gamestate.Pilot{Name: "A"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	v.SetPaper(1)
	before := v.IntelligenceText()
	for i := 0; i < 10; i++ {
		v.PaperFrame(1)
		if v.IntelligenceText() != before {
			t.Fatal("redraw rerolled visibility")
		}
	}
}

func TestBriefingSourcePreparedWhenRequested(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("set GS2000_TEST_MISSIONS for proprietary-source render integration")
	}
	a, err := assetdata.LoadMissionAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "Test Pilot Name"
	p.Squadron = "Test Squadron Name"
	p.Theatre = gamestate.TheatrePersianGulf
	m, err := gamestate.GenerateMission(a.Generation, gamestate.MissionInputs{MissionType: gamestate.MissionTraining, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewBriefingView(a, m, p, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.OrdersText(), "11/14") || !strings.Contains(v.OrdersText(), "BTR60 Platoon") || !strings.Contains(v.IntelligenceText(), m.Intelligence.GroundNames[0]) {
		t.Fatal("papers are not derived from source mission", v.OrdersText())
	}
	// The legend is in the selected world map; HANDS' same extent is zero.
	legendInk := 0
	for y := 132; y < 154; y++ {
		for x := 0; x < 165; x++ {
			if a.Images[m.Map][y*320+x] != 0 {
				legendInk++
			}
			if a.Images["HANDS.PIC"][y*320+x] != 0 {
				t.Fatal("source HANDS witness changed")
			}
		}
	}
	if legendInk == 0 {
		t.Fatal("selected source map has no legend")
	}
	before := m.ObjectHash
	frames := map[string][]byte{"briefing": v.Frame()}
	for i, name := range []string{"orders", "intel", "map", "decline"} {
		v.SetPaper(i)
		frames[name] = v.PaperFrame(i)
	}
	for _, name := range []string{"briefing", "orders", "intel", "map", "decline"} {
		f := frames[name]
		if len(f) != 64000 {
			t.Fatal(name, len(f))
		}
		t.Logf("%s indices SHA256 %s", name, assetdata.Hash(f))
	}
	if m.ObjectHash != before {
		t.Fatal("render mutated mission")
	}
	if out := os.Getenv("GS2000_TEST_BRIEFING_OUTPUT"); out != "" {
		if err = os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"briefing", "orders", "intel", "map", "decline"} {
			f, er := os.OpenFile(filepath.Join(out, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if er != nil {
				t.Fatal(er)
			}
			img := image.NewPaletted(image.Rect(0, 0, 320, 200), a.Palettes["FIELD.PAL"])
			copy(img.Pix, frames[name])
			er = png.Encode(f, img)
			ce := f.Close()
			if er != nil {
				t.Fatal(er)
			}
			if ce != nil {
				t.Fatal(ce)
			}
		}
	}
}

func TestBriefingInterstitialSourceSceneTableAndRemapScope(t *testing.T) {
	original := make([]byte, 64000)
	copy(original, []byte{0x44, 0x45, 0x4f, 0x50, 0xbf, 0xc0, 0xdf, 0xe0})
	a := &assetdata.MissionAssets{Images: map[string][]byte{"SHIP.PIC": original}}
	for scene, want := range []string{"TENT", "TENT", "SHIP", "TENT", "ICE", "SHIP"} {
		v := &BriefingView{Assets: a, Mission: &gamestate.MissionData{Scene: scene}}
		if v.InterstitialName() != want {
			t.Fatal(scene, v.InterstitialName(), want)
		}
	}
	v := &BriefingView{Assets: a, Mission: &gamestate.MissionData{Scene: 5}}
	out := v.InterstitialFrame()
	want := []byte{0x44, 232, 250, 0x50, 0xbf, 0xe0, 0xff, 0xe0}
	if !bytes.Equal(out[:8], want) {
		t.Fatal(out[:8], want)
	}
	if original[1] != 0x45 {
		t.Fatal("modified source atlas")
	}
}
