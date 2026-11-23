package assets

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"strings"
	"testing"
)

func sourceNDebrief(t *testing.T) debriefMetadata {
	t.Helper()
	root := testfixture.Path("installation/")
	gs, e := os.ReadFile(root + "GS.GS2")
	if os.IsNotExist(e) {
		t.Skip("optional original profile unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	cat, e := os.ReadFile(root + "GS2000.DAT")
	if e != nil {
		t.Fatal(e)
	}
	entries, e := parseDirectory(cat)
	if e != nil {
		t.Fatal(e)
	}
	r := map[string]*Resource{}
	for _, v := range entries {
		r[v.NameReadable] = &Resource{name: v.NameReadable, Payload: cat[v.Offset:v.End], Source: SourceLocation{Path: "GS2000.DAT", Kind: "archive_entry", PayloadSHA256: Hash(cat[v.Offset:v.End])}}
	}
	g := &Resource{name: "GS.GS2", Payload: gs, Source: SourceLocation{Path: "GS.GS2", Kind: "loose_file", SHA256: Hash(gs)}}
	d, e := prepareDebriefMetadata(r, g, &IslandIce469085)
	if e != nil {
		t.Fatal(e)
	}
	// Original four pushes at source264d..2683: both shoulder selectors are
	// rankbase+phase; x254 is a destination, not a selector as old pseudocode suggested.
	if !bytes.Equal(gs[0x369c0+0x265b:0x369c0+0x265b+4], []byte{0x6a, 0x13, 0x68, 0xfe}) {
		t.Fatal("JUMP high-rank source operands changed")
	}
	return d
}

func TestDebriefSourceTablesAndSceneABI(t *testing.T) {
	d := sourceNDebrief(t)
	if d.Text[0x4d52] != "Press a key to Continue." || d.Text[0x51c4] != "Press a key to Continue." || d.Text[0x525a] != "Press any key to Continue." {
		t.Fatal("source literals")
	}
	if d.RankNames[0] != "Warrant Officer Candidate" || d.RankNames[11] != "Brigadier General" {
		t.Fatal(d.RankNames)
	}
	if d.MedalNames[0] != "CMOH" || d.MedalNames[7] != "NDS" {
		t.Fatal(d.MedalNames)
	}
	if d.Scenes["HAPPY.SCN"].Background != "HAPPY_2.PIC" || d.Scenes["SAD.SCN"].Background != "SAD_2.PIC" || d.Scenes["SAD.SCN"].Palette != "SAD_2.PAL" {
		t.Fatal("selected VGA feedback graphics branch")
	}
	if len(d.Scenes["JUMP.SCN"].Lori) != 8 || d.Scenes["JUMP.SCN"].Lori[0].Mode != 1 || d.Scenes["JUMP.SCN"].Lori[7].SourceAsset != "RANKS.PIC" {
		t.Fatal("semantic atlas/mask ownership")
	}
	if d.Feedback["success"][0] != "Great mission!  You were right on the mark - they never knew what hit'em." || len(d.Feedback["partial"]) != 8 {
		t.Fatal("exact far-string pointer table")
	}
	a := &DebriefAssets{Text: d.Text, RankNames: d.RankNames, MedalCitations: d.MedalCitations, WingLevels: d.WingLevels, WingQualifiers: d.WingQualifiers, Feedback: d.Feedback}
	scene, text := a.FeedbackPage(DebriefFeedbackInput{Success: true, MissingObjectives: true, NotReturned: true, Variant: 4})
	if scene != "HAPPY.SCN" || !strings.HasPrefix(text, "One out of two isn't bad") || !strings.Contains(text, "returning to base without their helicopters") {
		t.Fatal(scene, text)
	}
	p := a.JumpParagraphs(DebriefJumpInput{Success: true, Elapsed: 360 * 31})
	if len(p) != 1 || p[0] != d.Text[0x5311] {
		t.Fatal(p)
	}
	p = a.JumpParagraphs(DebriefJumpInput{Success: true, Elapsed: 360 * 31, SpecialOutcome: true})
	if len(p) != 1 || p[0] != d.Text[0x5478] {
		t.Fatal("861c must suppress time text independently of commander", p)
	}
	if DebriefSurname("A B. Jones") != "Jones" || DebriefSurname("Solo") != "Solo" {
		t.Fatal("source surname suffix")
	}
}

func TestDebriefPreparedOnlyLoad(t *testing.T) {
	root := os.Getenv("GS2000_TEST_PREPARED")
	if root == "" {
		t.Skip("set GS2000_TEST_PREPARED for prepared-only debrief integration")
	}
	a, e := LoadFrontEndAssets(root)
	if e != nil {
		t.Fatal(e)
	}
	if a.Debrief == nil || len(a.Debrief.Images) != len(debriefImageNames) || len(a.Debrief.Scenes) != len(debriefSceneNames) {
		t.Fatal("prepared debrief incomplete")
	}
	d := a.Debrief
	frame := append([]byte(nil), d.Images["JUMP.PIC"]...)
	for _, rank := range []int{0, 1, 6, 7, 11} {
		for phase := 0; phase < 6; phase++ {
			if e = d.DrawJumpInsignia(frame, rank, 3, phase, 0, [8]bool{false, true, false, false, false, false, false, true}); e != nil {
				t.Fatal(rank, phase, e)
			}
		}
	}
	for _, scene := range []string{"SAD.SCN", "HAPPY.SCN"} {
		r := d.Scenes[scene].Areas[1].Rectangle
		if e = d.Lori(frame, scene, 0, 2, r[0], r[1]); e != nil {
			t.Fatal(e)
		}
	}
}

func TestDebriefMissingFeedbackFooterRejected(t *testing.T) {
	d := sourceNDebrief(t)
	delete(d.Text, 0x51c4)
	if err := validateDebriefMetadata(d, false); err == nil || !strings.Contains(err.Error(), "51c4") {
		t.Fatal("missing source footer must not pass preparation/readiness", err)
	}
}
