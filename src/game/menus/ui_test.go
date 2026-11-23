package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func originalGS(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original profile unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != "46ce1a52b67e50ebee7d5aacafa063021bfd7e39303b5a67da61a5cd5aed90e9" {
		t.Fatal("unrecognized original GS profile")
	}
	return b
}

func sourceOperand(t *testing.T, b []byte, at int, raw string) {
	t.Helper()
	want, err := hex.DecodeString(raw)
	if err != nil || !bytes.Equal(b[at:at+len(want)], want) {
		t.Fatalf("original source operand at file%x differs", at)
	}
}

func preparedUI(t *testing.T) *assetdata.OfficeAssets {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("source-backed prepared UI opt in")
	}
	a, err := assetdata.LoadFrontEndAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDebriefOriginalBankArgumentsAndPreparedSceneColours(t *testing.T) {
	b := originalGS(t)
	// Section3 file369c0 + IP. Seven literal pushes encode
	// fill0,height172,width,x0,y0,style,bank08a4, before D02:0c4e.
	sourceOperand(t, b, 0x369c0+0x240, "6a0068ac006818016a006a006a0068a4089a4e0c020d")
	sourceOperand(t, b, 0x369c0+0x1e83, "6a0068ac006a706a006a006a0468a4089a4e0c020d")
	sourceOperand(t, b, 0x369c0+0x1a4c, "6a0068ac006818016a006a006a0068a4089a4e0c020d")
	// Cache initialization samples POINTER y105 into DS85a8+12*style.
	sourceOperand(t, b, 0xfa35, "6a698b46fe8bc8c1e00203c10346fc506a01")
	sourceOperand(t, b, 0xfa5e, "8880a885")
	a := preparedUI(t)
	for _, q := range []struct {
		palette string
		style   int
		index   byte
		rgb6    [3]byte
	}{{"COMM.PAL", 0, 234, [3]byte{19, 19, 19}}, {"JUMP.PAL", 4, 26, [3]byte{20, 20, 20}}, {"HAPPY.PAL", 0, 234, [3]byte{19, 19, 19}}, {"SAD_2.PAL", 0, 234, [3]byte{19, 19, 20}}} {
		if got := a.Missions.Images["POINTER.PIC"][105*320+5*q.style]; got != q.index {
			t.Fatalf("%s style%d source body index%d", q.palette, q.style, got)
		}
		r, g, bl, _ := a.Debrief.Palettes[q.palette][q.index].RGBA()
		// Prepared RGB6 expands through the established 6-to-8 bit mapping.
		for i, v := range []uint32{r, g, bl} {
			want := uint32(q.rgb6[i]<<2 | q.rgb6[i]>>4)
			if v>>8 != want {
				t.Fatalf("%s body channel%d=%d want%d", q.palette, i, v>>8, want)
			}
		}
	}
}

func TestDebriefBanksKeepGlyphHolesBodyFrameAndClipping(t *testing.T) {
	a := preparedUI(t)
	c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus}
	font := a.Menus.Fonts[3]
	var glyph assetdata.FontGlyph
	for _, g := range font.Metrics.Glyphs {
		if g.Byte == 'O' {
			glyph = g
		}
	}
	for _, q := range []struct {
		x, width, style int
		body            byte
	}{{16, 280, 0, 234}, {6, 112, 4, 26}} {
		frame := bytes.Repeat([]byte{253}, 64000)
		c.drawDebriefBody(frame, "O", q.x, q.width, q.style)
		on, holes := 0, 0
		for y := 0; y < font.Metrics.Height; y++ {
			for x := 0; x < glyph.Advance; x++ {
				want := q.body
				if font.Mask.ColorIndexAt(glyph.Rect[0]+x, glyph.Rect[1]+y) != 0 {
					want = 10
					on++
				} else {
					holes++
				}
				if got := frame[(170+y)*320+q.x+4+x]; got != want {
					t.Fatalf("style%d glyph%d,%d=%d want%d", q.style, x, y, got, want)
				}
			}
		}
		if on == 0 || holes == 0 || frame[177*320+q.x+4+q.width-1] != q.body {
			t.Fatal("missing independent ink/hole/blank body coverage")
		}
		pointer := a.Missions.Images["POINTER.PIC"]
		if frame[181*320+q.x+2] != pointer[104*320+5*q.style+2] {
			t.Fatal("source lower bevel replaced by text bank")
		}
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				if x >= q.x && x < q.x+q.width+8 && y >= 166 && y <= 181 {
					continue
				}
				if frame[y*320+x] != 253 {
					t.Fatalf("body changed scene/footer outside source copy at%d,%d", x, y)
				}
			}
		}
		frame = bytes.Repeat([]byte{253}, 64000)
		c.drawDebriefBody(frame, strings.Repeat("O\n", 26)+"O", q.x, q.width, q.style)
		if frame[199*320+q.x+4] != 253 || frame[182*320+q.x+4] != 253 || frame[q.x-1] != 253 {
			t.Fatal("top-clipped long bank grew into footer or surrounding scene")
		}
	}
	// Exercise the reached page selectors themselves, including the commander
	// default that older rendering tests incorrectly assumed to be style4.
	before := &gamestate.Pilot{Name: "SOURCE TEST", Rank: gamestate.RankWOC, LegacyRaw: make([]byte, gamestate.LegacyRecordSize), Statistics: &gamestate.PilotStatistics{AwardCounts: make([]int8, 8)}}
	pilot := *before
	pilot.LegacyRaw = append([]byte(nil), before.LegacyRaw...)
	pilot.LegacyRaw[0x55] = 1
	proposal := &gamestate.SortieProposal{PilotBefore: before, CommanderRank: 1, Success: true}
	c.Debrief = &DebriefView{Pending: &gamestate.PendingSortie{Proposal: proposal}, Pilot: &pilot}
	for _, q := range []struct {
		page     string
		x, style int
	}{{"commander", 16, 0}, {"jump", 6, 4}, {"feedback", 16, 0}} {
		c.enterDebriefPage(q.page)
		c.Debrief.Tick = 120
		frame := c.debriefFrame()
		if frame[181*320+q.x+2] != a.Missions.Images["POINTER.PIC"][104*320+5*q.style+2] {
			t.Fatalf("%s did not use source style%d", q.page, q.style)
		}
		if output := os.Getenv("GS2000_TEST_N1_UI_OUTPUT"); output != "" {
			palette := a.Debrief.Scenes[c.Debrief.Scene].Palette
			if err := writeIndexedFrame(filepath.Join(output, q.page+".png"), frame, a.Debrief.Palettes[palette], a.Root, map[string]any{"page": q.page, "style": q.style, "palette": palette, "scope": "supplied native state; source-backed panel fill, not original-frame equivalence"}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestOrdnanceOriginalDescriptionAndRowColourDomains(t *testing.T) {
	b := originalGS(t)
	// ORDNANCE1d50:IP -> file33ae0+IP, retained Step37 mapping.
	for ip, raw := range map[int]string{0x90c: "263847057e50", 0x935: "c646fc0c", 0x983: "c646fc0f", 0x174f: "c646a303c646a40ac646a50f", 0x1a14: "8a470898509ab800870c"} {
		sourceOperand(t, b, 0x33ae0+ip, raw)
	}
	a := preparedUI(t)
	for at, want := range map[int]string{0x3eb5: "The Longbow Apache is restricted, your rank must be Captain or above.", 0x3efb: "The Comanche is restricted, your rank must be 1st Lieut. or above."} {
		if got := a.Missions.Preflight.Text[at]; got != want {
			t.Fatalf("source restriction DS%x=%q want%q", at, got, want)
		}
	}
	m := &gamestate.MissionData{SourceWorld: &a.Missions.Generation.Worlds[0], Conditions: gamestate.MissionConditions{Temperature: 68}}
	for _, q := range []struct {
		id, rank, ink int
		denied        bool
	}{{1, 1, 12, true}, {1, 7, 15, false}, {6, 5, 12, true}, {6, 6, 15, false}, {7, 5, 12, true}, {7, 6, 15, false}} {
		row, err := gamestate.InitializeReadinessRow(a.Missions, m, 2)
		if err != nil {
			t.Fatal(err)
		}
		rank, err := gamestate.RankFromRaw(q.rank)
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewOrdnanceView(a.Missions, m, []gamestate.FlightPilot{{Name: "SOURCE TEST"}}, [][36]byte{row}, 0, rank)
		if err != nil {
			t.Fatal(err)
		}
		ordnanceActivate(t, v, "aircraft", -1)
		index := -1
		for i, choice := range v.Choices {
			if choice.Action == "replace" && choice.Value == q.id {
				index = i
			}
		}
		if index < 0 || v.Choices[index].Enabled == q.denied {
			t.Fatal("source rank row missing or wrong eligibility", q)
		}
		v.Hover = index
		before, live := v.Working, v.Rows[0]
		frame, plain := v.FrameForHit(index), v.FrameForHit(-1)
		choice := v.Choices[index]
		font := a.Missions.Fonts[3]
		// Every source mask bit of the row uses normal10/hover15, including
		// rank-denied rows. Compare masks, not a regenerated screenshot golden.
		for i, line := range ordnanceWrap(font, choice.Text, 76) {
			mask := make([]byte, 64000)
			_ = font.DrawClipped(mask, []byte(line), 237, choice.Rectangle[1]+3+i*6, 1, 0, 0, [4]int{229, 67, 91, 110})
			for at, bit := range mask {
				if bit != 0 && (frame[at] != 15 || plain[at] != 10) {
					t.Fatalf("rank row uses availability as list ink at%d", at)
				}
			}
		}
		body := preflightCString(a.Missions.AircraftDefinitions[q.id], 131)
		if q.denied {
			at := 0x3efb
			if q.id == 1 {
				at = 0x3eb5
			}
			body = a.Missions.Preflight.Text[at]
		}
		if body == "" {
			t.Fatal("source description literal missing")
		}
		mask := make([]byte, 64000)
		for i, line := range ordnanceWrap(font, body, 80) {
			_ = font.DrawClipped(mask, []byte(line), 233, 33+i*6, 1, 0, 0, [4]int{233, 33, 80, 35})
		}
		ink := 0
		for at, bit := range mask {
			if bit != 0 {
				ink++
				if frame[at] != byte(q.ink) {
					t.Fatalf("upper description ink at%d=%d want%d", at, frame[at], q.ink)
				}
			}
		}
		if ink == 0 || v.Working != before || v.Rows[0] != live {
			t.Fatal("hover changed equipment or omitted description")
		}
		if output := os.Getenv("GS2000_TEST_N1_UI_OUTPUT"); output != "" {
			if err := writeIndexedFrame(filepath.Join(output, fmt.Sprintf("aircraft%d-rank%d.png", q.id, q.rank)), frame, a.Missions.Palettes["FIELD.PAL"], a.Root, map[string]any{"aircraft": q.id, "rank": q.rank, "rank_denied": q.denied, "body_ink": q.ink, "scope": "supplied native state; source-backed description/row domains, not original-frame equivalence"}); err != nil {
				t.Fatal(err)
			}
		}
		v.audioCues = nil
		if _, err := v.Activate(index); err != nil {
			t.Fatal(err)
		}
		if q.denied {
			if v.Working != before || v.Rows[0] != live || v.Message != "" || v.Hover != index || !bytes.Equal(frame[191*320:], v.FrameForHit(index)[191*320:]) || len(v.audioCues) != 2 || v.audioCues[0] != 0x22 || v.audioCues[1] != 0x21 {
				t.Fatal("denied activation changed loadout, hover, footer or ordered cues")
			}
		} else if v.Working[0] != byte(q.id) || v.Rows[0] != v.Working {
			t.Fatal("allowed replacement did not initialize committed aircraft")
		}
		m.CampaignUnavailableAircraft = []int{q.id}
		v.rebuild()
		for _, choice := range v.Choices {
			if choice.Action == "replace" && choice.Value == q.id {
				t.Fatal("supplied Campaign exclusion rendered as rank-disabled row")
			}
		}
		m.CampaignUnavailableAircraft = nil
	}
}
