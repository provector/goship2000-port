package assets

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestDebriefOriginalDescriptorsAndControllerOperands(t *testing.T) {
	d := sourceNDebrief(t)
	// Preparation28 admits the independently recovered REPLACE descriptor.
	// The earlier nine-scene/schema2 profile remains a separate reader case.
	if len(d.Scenes) != 10 || len(d.Scenes["RESCUE.SCN"].Components) != 197 {
		t.Fatal("source scene coverage")
	}
	k := d.Scenes["KIA.SCN"]
	if k.Background != "KIA.PIC" || k.Palette != "KIA.PAL" || k.Areas[1].Rectangle != [4]int{223, 104, 34, 42} || k.Lori[0].Cell != [2]int{34, 42} || k.Lori[0].Divisor != 4 || k.Lori[0].Tail != 20 || k.Lori[0].Mode != 1 {
		t.Fatal("original bugler rectangle/grid", k)
	}
	r := d.Scenes["RESCUE.SCN"].Components
	if r[0] != (DebriefComponent{Group: 0, SourceAsset: "RESC_AN.PIC", Source: [4]int{0, 0, 36, 28}, Destination: [2]int{24, 118}}) || r[len(r)-1].Group != 80 {
		t.Fatal("original CMPX first/last groups")
	}
	c := d.Scenes["CREWPRO.SCN"]
	if c.Background != "" || c.Palette != "FIELD.PAL" || c.Lori[0].Base != [2]int{220, 120} || c.Lori[0].Cell != [2]int{34, 14} || c.Lori[2].SourceAsset != "PATCHES.PIC" {
		t.Fatal("source crew atlases", c)
	}
	q := d.Scenes["REPLACE.SCN"]
	if q.Background != "FIELD.PIC" || q.Palette != "FIELD.PAL" || len(q.Areas) != 2 || q.Areas[1].Rectangle != [4]int{71, 3, 233, 187} || len(q.Lori) != 1 || q.Lori[0].SourceAsset != "MEMO.PIC" || q.Lori[0].Base != [2]int{240, 135} || q.Lori[0].Cell != [2]int{67, 57} || q.Lori[0].Mode != 1 || q.Lori[0].Divisor != 1 || q.Lori[0].Tail != 1 || q.Source.PayloadSHA256 != "4c13440633f17a33bcab78d84f3fae818d337282b7f1038779c38b4517529b9d" {
		t.Fatal("source REPLACE descriptor", q)
	}
	if d.Text[0x5b82] != "FIELD" || d.Text[0x5b88] != "REPLACE" || d.Text[0x5b90] != "PILOT" || d.Text[0x5f4e] != "SELECTOR #1 to Accept.  SELECTOR #2 to rename pilot." {
		t.Fatal("source replacement names/footer")
	}
	for _, name := range []string{"WIN.SCN", "LOSE.SCN"} {
		if d.Scenes[name].Areas[0].Rectangle != [4]int{0, 0, 320, 191} {
			t.Fatal("conclusion source AREA")
		}
	}
	if d.Text[0x589b] != " - posthumously." || d.Text[0x5134] != "NONE" || d.Text[0x5139] != "ACCEPT" {
		t.Fatal("loss/crew exact source strings")
	}
	gs, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	// Verified original stack pushes: style5 belongs to KIA's200-wide bank;
	// RESCUE is300-wide/style0. These are not renderer-generated expectations.
	for _, q := range []struct {
		at  int
		raw []byte
	}{
		{0x369c0 + 0x148f, []byte{0x6a, 0, 0x68, 0xac, 0, 0x68, 0xc8, 0, 0x6a, 0, 0x6a, 0, 0x6a, 5}},
		{0x369c0 + 0x2fc7, []byte{0x6a, 0, 0x68, 0xac, 0, 0x68, 0x2c, 1, 0x6a, 0, 0x6a, 0, 0x6a, 0}},
		{0x1b53b - 0xe000, []byte{0x6a, 0, 0x68, 0xac, 0, 0x68, 0x18, 1, 0x6a, 0, 0x6a, 0, 0x6a, 0}},
		// REPLACE marks cache bank2, not font2; row font4 is an independent
		// c87:0110 call. Name helper15 is erase ink/max10/height7.
		{0x369c0 + 0x29cc, []byte{0x6a, 2, 0x9a, 0x8c, 6, 2, 0x0d}},
		{0x369c0 + 0x2b18, []byte{0x6a, 4, 0x9a, 0x10, 1, 0x87, 0x0c}},
		{0x369c0 + 0x28eb, []byte{0x6a, 15, 0x6a, 10, 0x6a, 7}},
		{0x369c0 + 0x2a3b, []byte{0x6a, 13, 0x68, 0xf7, 0}},
	} {
		if !bytes.Equal(gs[q.at:q.at+len(q.raw)], q.raw) {
			t.Fatalf("GS source graphics operands at%x", q.at)
		}
	}
}

func TestDebriefNewAndHistoricalSchemasRemainStrict(t *testing.T) {
	d := sourceNDebrief(t)
	if d.Schema != 4 {
		t.Fatal("source feedback picture admission must be schema4")
	}
	if err := validateDebriefMetadata(d, false); err != nil {
		t.Fatal(err)
	}
	clone := func() debriefMetadata {
		b, e := json.Marshal(d)
		if e != nil {
			t.Fatal(e)
		}
		var q debriefMetadata
		if e = json.Unmarshal(b, &q); e != nil {
			t.Fatal(e)
		}
		return q
	}
	// Schema3 retains the exact REPLACE/ten-scene reader. Schema4 adds only
	// the plain/Arctic image and SAD-palette admissions in prepared outputs.
	historical3 := clone()
	historical3.Schema = 3
	if err := validateDebriefMetadata(historical3, false); err != nil {
		t.Fatal("historical schema3 reader changed", err)
	}
	for _, kind := range []string{"extra-scene", "renamed-scene", "missing-replace-prompt", "unknown-schema"} {
		t.Run(kind, func(t *testing.T) {
			q := clone()
			switch kind {
			case "extra-scene":
				q.Scenes["UNSUPPORTED.SCN"] = q.Scenes["REPLACE.SCN"]
			case "renamed-scene":
				q.Scenes["UNSUPPORTED.SCN"] = q.Scenes["REPLACE.SCN"]
				delete(q.Scenes, "REPLACE.SCN")
			case "missing-replace-prompt":
				delete(q.Text, 0x5f4e)
			case "unknown-schema":
				q.Schema = 5
			}
			if validateDebriefMetadata(q, false) == nil {
				t.Fatal("unsupported metadata accepted", kind)
			}
		})
	}
	// Contract-shape negative: new data cannot be smuggled into a historical
	// nine-scene interpretation simply by changing its schema number.
	old := clone()
	old.Schema = 2
	if validateDebriefMetadata(old, false) == nil {
		t.Fatal("schema2 accepted later scene")
	}
	delete(old.Scenes, "REPLACE.SCN")
	if err := validateDebriefMetadata(old, false); err != nil {
		t.Fatal("historical nine-scene interpretation rejected", err)
	}
	old.Scenes["UNSUPPORTED.SCN"] = d.Scenes["REPLACE.SCN"]
	delete(old.Scenes, "WIN.SCN")
	if validateDebriefMetadata(old, false) == nil {
		t.Fatal("historical count alone admitted unknown scene")
	}
}

func TestHistoricalSchema2DebriefPreparedReadback(t *testing.T) {
	root := os.Getenv("GS2000_TEST_O1_ASSETS")
	if root == "" {
		t.Skip("explicit historical Preparation25 fixture required")
	}
	a, err := LoadFrontEndAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	if a.Debrief == nil || len(a.Debrief.Scenes) != 9 || len(a.Debrief.Images) != 20 {
		t.Fatal("historical schema2 media/scene domain changed")
	}
	if _, ok := a.Debrief.Scenes["REPLACE.SCN"]; ok {
		t.Fatal("historical profile acquired new REPLACE interpretation")
	}
	if _, ok := a.Debrief.Images["MEMO.PIC"]; ok {
		t.Fatal("historical debrief bundle silently gained current media")
	}
}

func TestDebriefSourceTextAndIndependentVariants(t *testing.T) {
	d := sourceNDebrief(t)
	a := &DebriefAssets{Text: d.Text, Feedback: d.Feedback}
	if a.RescueParagraph("", 1) != "Fortune smiled on you this time. The search & rescue guys were able to locate your distress beacon." {
		t.Fatal("generic SAR wording")
	}
	if a.RescueParagraph("Adams", 1) != "The breaks fell your way!  Adams was near enough to snap you up with his Blackhawk.  You lead on." {
		t.Fatal("named SAR source template")
	}
	if a.CampaignParagraph(true) != d.Text[0x36e] || a.CampaignParagraph(false) != d.Text[0x2e8] {
		t.Fatal("paragraph source success selector")
	}
	_, text := a.FeedbackPage(DebriefFeedbackInput{Success: true, MissingObjectives: true, NotReturned: true, CampaignWarning: true, Variant: 999, Variants: map[string]uint32{"partial": 2, "campaign-warning": 1, "no-return": 3}})
	want := d.Feedback["partial"][2] + d.Feedback["campaign-warning"][1] + d.Feedback["no-return"][3]
	if text != want {
		t.Fatal("independent source draws collapsed to a shared modulo", text, want)
	}
}
