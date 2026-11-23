package menus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"testing"
)

// Display/input fixture only. Earned commission/result publication is covered
// separately through the real mission/finalack flow, not manufactured here.
func replacementDisplay(t *testing.T) *FrontEnd {
	t.Helper()
	c := debriefDisplay(t)
	p := c.Debrief.Pending.Proposal
	p.Version = 4
	p.Result.ReplacementCount = 4
	p.Result.ReplacementInitial = true
	p.Result.CrewPro = false
	p.Result.Replacements = make([]gamestate.SquadronPilot, 4)
	c.Debrief.Pilot.SquadronPilots = make([]gamestate.SquadronPilot, 4)
	for i, name := range []string{"ALPHA", "BRAVO", "CHARLIE", "DELTA"} {
		raw := make([]byte, 41)
		copy(raw, name)
		raw[33], raw[34], raw[35], raw[36] = 1, 1, 1, byte(i+1)
		row := gamestate.SquadronPilot{Name: name, Rank: gamestate.RankWO1, Portrait: i + 1, LegacyRaw: raw}
		c.Debrief.Pilot.SquadronPilots[i], p.Result.Replacements[i] = row, row
	}
	c.enterDebriefPage("replace")
	return c
}

func TestPendingReplacementUIAndHeldEnter(t *testing.T) {
	a := preparedUI(t)
	b, err := os.ReadFile(testfixture.Path("commission-sortie.json"))
	if os.IsNotExist(err) {
		t.Skip("retained earned Single context unavailable; mandatory release manifest supplies this fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "9d374136c6f29a2a63701a261278bcaf5e4cd87670dce7a18c2cb1be31e3a75d" {
		t.Fatal("retained journal identity differs")
	}
	var saved struct {
		Start    gamestate.SortieStart     `json:"start"`
		Proposal *gamestate.SortieProposal `json:"proposal"`
	}
	if err = json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	s, err := gamestate.OpenSaveStore(filepath.Join(t.TempDir(), "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	next := s.State()
	next.Slots[0], next.SelectedPilotID = saved.Start.PilotBefore, saved.Start.PilotBefore.ID
	if err = s.Commit(next); err != nil {
		t.Fatal(err)
	}
	request := saved.Start
	request.ID = ""
	start, err := s.BeginSortie(request)
	if err != nil {
		t.Fatal(err)
	}
	facts := saved.Proposal.Facts
	facts.SortieID = start.ID
	context := *facts.ResultContext
	context.Seed = gamestate.ResultSeed(start.ID)
	facts.ResultContext = &context
	proposal, err := gamestate.ProposeSortie(start.PilotBefore, facts, saved.Proposal.Tables)
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Commission || proposal.Score.Score != 1039 {
		t.Fatal("retained arithmetic control not reached")
	}
	if err = s.StageSortieResult(start.ID, proposal); err != nil {
		t.Fatal(err)
	}
	c := &FrontEnd{Office: &Office{Assets: a}, Menus: a.Menus, Store: s}
	if err = c.enterSortieDebrief(); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "commander" {
		t.Fatal("missing offered commission")
	}
	var latch inputLatch
	c.Debrief.Tick = 400
	if err = c.handleDebrief(latch.Read(Buttons{Enter: true})); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "jump" {
		t.Fatal("accepted commander did not reach JUMP")
	}
	c.Debrief.Tick = 400
	if err = c.handleDebrief(latch.Read(Buttons{Enter: true})); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "jump" {
		t.Fatal("held Enter crossed a page")
	}
	_ = latch.Read(Buttons{})
	if err = c.handleDebrief(latch.Read(Buttons{Enter: true})); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "replace" {
		t.Fatal("earned-context commission did not reach REPLACE")
	}
	_ = latch.Read(Buttons{})
	if err = c.handleDebrief(latch.Read(Buttons{Backspace: true})); err != nil {
		t.Fatal(err)
	}
	if !c.Debrief.ReplaceEditing {
		t.Fatal("source rename input not reached")
	}
	if err = c.handleDebrief(Actions{Text: []rune("NEW NAME  ")}); err != nil {
		t.Fatal(err)
	}
	_ = latch.Read(Buttons{})
	if err = c.handleDebrief(latch.Read(Buttons{Enter: true})); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.ReplaceEditing || c.Debrief.Page != "replace" || c.Debrief.Pilot.SquadronPilots[0].Name != "NEW NAME" {
		t.Fatal("source accepted/trimmed surname not resolved")
	}
	if err = c.handleDebrief(latch.Read(Buttons{Enter: true})); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "replace" {
		t.Fatal("held rename Enter acknowledged the outer page")
	}
	canonical := s.State().SelectedPilot().SquadronPilots
	if len(canonical) > 0 && canonical[0].Name == "NEW NAME" {
		t.Fatal("pending rename wrote canonical career")
	}
	if err = c.handleReplacement(Actions{Down: true, Backspace: true}); err != nil {
		t.Fatal(err)
	}
	if err = c.handleReplacement(Actions{Text: []rune("CANCEL")}); err != nil {
		t.Fatal(err)
	}
	if err = c.handleReplacement(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Pilot.SquadronPilots[1].Name == "CANCEL" {
		t.Fatal("Escape committed temporary surname")
	}
	if err = s.CommitSortie(start.ID); err != nil {
		t.Fatal(err)
	}
	if s.State().SelectedPilot().SquadronPilots[0].Name != "NEW NAME" {
		t.Fatal("final acknowledgement lost accepted surname")
	}
}

func TestReplaceSourceSceneAndInputIsolation(t *testing.T) {
	c := replacementDisplay(t)
	a := c.Office.Assets.Debrief
	s := a.Scenes["REPLACE.SCN"]
	if s.Background != "FIELD.PIC" || s.Palette != "FIELD.PAL" || len(s.Areas) != 2 || s.Areas[1].Rectangle != [4]int{71, 3, 233, 187} || len(s.Lori) != 1 || s.Lori[0].SourceAsset != "MEMO.PIC" {
		t.Fatalf("source REPLACE scene %+v", s)
	}
	areas, lines, advance := c.replacementLayout()
	if areas[0][2] != 200 || areas[0][3] != 8 || areas[3][1]+8 > 190 || len(lines) == 0 || advance <= 0 {
		t.Fatal("source row/paragraph bounds", areas, len(lines), advance)
	}
	t.Logf("source paragraph %d lines, advance%d, row AREAs %v", len(lines), advance, areas)
	before, _ := json.Marshal(c.Debrief.Pending)
	for n := 0; n < 3; n++ {
		frame := c.replacementFrame()
		if len(frame) != 64000 {
			t.Fatal("frame size")
		}
		for y := 0; y < 190; y++ {
			if !bytes.Equal(frame[y*320:y*320+70], a.Images["MEMO.PIC"][y*320:y*320+70]) {
				t.Fatal("common MEMO drawable bank lost outside the paper", y)
			}
		}
		if !bytes.Equal(frame[191*320:193*320], make([]byte, 640)) {
			t.Fatal("source footer black band")
		}
	}
	after, _ := json.Marshal(c.Debrief.Pending)
	if !bytes.Equal(before, after) {
		t.Fatal("drawing changed pending rows, choices or result RNG")
	}
	// A parked pointer over row0 must not undo keyboard-selected row1.
	if err := c.handleReplacement(Actions{Down: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.handleReplacement(Actions{Inside: true, X: areas[0][0] + 2, Y: areas[0][1] + 2}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.ReplaceRow != 1 {
		t.Fatal("parked mouse overrode keyboard")
	}
	if err := c.handleReplacement(Actions{Inside: true, PointerMoved: true, X: areas[2][0] + 2, Y: areas[2][1] + 2}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.ReplaceRow != 2 {
		t.Fatal("physical pointer did not select source row")
	}
	if err := c.handleReplacement(Actions{Backspace: true}); err != nil {
		t.Fatal(err)
	}
	if !c.Debrief.ReplaceEditing || c.Debrief.ReplaceBuffer != "" {
		t.Fatal("source helper must back up row and clear input buffer")
	}
	if err := c.handleReplacement(Actions{Text: []rune("ABCDEFGHIJK")}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.ReplaceBuffer != "ABCDEFGHIJ" {
		t.Fatal("source10-byte limit", c.Debrief.ReplaceBuffer)
	}
	if err := c.handleReplacement(Actions{Delete: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.ReplaceBuffer != "ABCDEFGHI" {
		t.Fatal("source Delete14b semantics")
	}
	if err := c.handleReplacement(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.ReplaceEditing || c.Debrief.Pilot.SquadronPilots[2].Name != "CHARLIE" {
		t.Fatal("Escape changed stored row")
	}
	if err := c.handleReplacement(Actions{Cancel: true}); err != nil || c.Debrief.Page != "replace" {
		t.Fatal("outer Escape accepted page", err)
	}
	// Real InputLatch emits both Enter and Selector2. The source key13 is
	// acceptance and must not accidentally open the key8 rename path.
	var latch inputLatch
	action := latch.Read(Buttons{Enter: true})
	if !action.Enter || !action.Selector2 {
		t.Fatal("fixture did not exercise real Enter alias")
	}
	if err := c.handleReplacement(action); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "feedback" {
		t.Fatal("key13 did not accept source replacement page")
	}
}

func TestReplaceCommonMemoSourceWindowFontAndNoNameHighlight(t *testing.T) {
	gs := originalGS(t)
	// Actual17d1:f6e thunk ->1b72:0022, normalized section0:0fb2.
	// Source copy/window/font/tab operands are not derived from the renderer.
	for _, x := range []struct {
		at    int
		bytes string
	}{
		{0x1ac7e, "e887f6ea2200721b0000"},
		{0x1dfa2 + 0x36, "6aff68b70068e0006a076a4d688008"},
		{0x1dfa2 + 0x4d, "6a049a1001870c"},
		{0x369c0 + 0x2b18, "6a049a1001870c"},
		{0x369c0 + 0x2a72, "6a0068ee006a006a009ada006a10"},
	} {
		sourceOperand(t, gs, x.at, x.bytes)
	}
	c := replacementDisplay(t)
	areas, lines, step := c.replacementLayout()
	if step != 8 || areas[0][1] != 47+(len(lines)-1)*8 {
		t.Fatal("actual memo cursor/line advance", areas, len(lines), step)
	}
	font := c.Menus.Fonts[4]
	rankMax := font.Measure([]byte("WO1"))
	if areas[0][0] != 77+rankMax+4 || areas[0][2] != 200 {
		t.Fatal("source rank/name AREA", areas[0], rankMax)
	}
	f := c.replacementFrame()
	expect := append([]byte(nil), c.Office.Assets.Debrief.Images["MEMO.PIC"]...)
	_ = font.DrawClipped(expect, []byte("TO:"), 77, 15, 0, 0, 0, [4]int{77, 7, 224, 183})
	for y := 15; y < 22; y++ {
		if !bytes.Equal(f[y*320+77:y*320+91], expect[y*320+77:y*320+91]) {
			t.Fatal("source memo header/font4", y)
		}
	}
	c.Debrief.ReplaceRow, c.Debrief.ReplacePortrait = 1, 1
	g := c.replacementFrame()
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			if x >= 247 && x < 295 && y >= 13 && y < 45 {
				continue
			}
			if f[y*320+x] != g[y*320+x] {
				t.Fatal("AREA change painted a name bar instead of only changing portrait", x, y)
			}
		}
	}
}

func TestReplacePointerCueSurvivesFullHandleUntilActualMouse(t *testing.T) {
	c := replacementDisplay(t)
	var err error
	c.Store, err = gamestate.OpenSaveStore(filepath.Join(t.TempDir(), "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the real predecessor: the OS pointer is parked in the memo
	// body when JUMP advances, independently of the new dynamic surname AREA.
	c.pointerX, c.pointerY, c.pointerInside = 135, 96, true
	c.Office.SetPointer(135, 96, true)
	c.enterDebriefPage("replace")
	areas, _, _ := c.replacementLayout()
	centre := func(i int) [2]int { r := areas[i]; return [2]int{r[0] + (r[2]-1)/2, r[1] + (r[3]-1)/2} }
	check := func(row int, point [2]int) {
		t.Helper()
		if c.Debrief.ReplaceRow != row || c.Debrief.ReplacePortrait != row || [2]int{c.Office.PointerX, c.Office.PointerY} != point || !c.Office.PointerInside {
			t.Fatalf("row%d point%v: selected%d portrait%d pointer%d,%d inside%v", row, point, c.Debrief.ReplaceRow, c.Debrief.ReplacePortrait, c.Office.PointerX, c.Office.PointerY, c.Office.PointerInside)
		}
	}
	check(0, centre(0))
	for i := 0; i < 8; i++ {
		if err = c.Handle(Actions{Inside: true, X: 135, Y: 96}); err != nil {
			t.Fatal(err)
		}
		c.Frame(false)
		check(0, centre(0))
	}
	if err = c.Handle(Actions{Inside: true, X: 135, Y: 96, Down: true}); err != nil {
		t.Fatal(err)
	}
	check(1, centre(1))
	for i := 0; i < 8; i++ {
		if err = c.Handle(Actions{Inside: true, X: 135, Y: 96}); err != nil {
			t.Fatal(err)
		}
		c.Frame(false)
		check(1, centre(1))
	}
	actual := [2]int{areas[2][0] + 2, areas[2][1] + 2}
	if err = c.Handle(Actions{Inside: true, X: actual[0], Y: actual[1]}); err != nil {
		t.Fatal(err)
	}
	if c.pointerCue != nil {
		t.Fatal("actual host movement did not release keyboard pointer cue")
	}
	check(2, actual)
	if err = c.Handle(Actions{Inside: true, X: actual[0], Y: actual[1]}); err != nil {
		t.Fatal(err)
	}
	check(2, actual)
	if err = c.Handle(Actions{Inside: true, X: actual[0], Y: actual[1], Up: true}); err != nil {
		t.Fatal(err)
	}
	check(1, centre(1))
	if err = c.Handle(Actions{Inside: true, X: actual[0], Y: actual[1], Backspace: true}); err != nil {
		t.Fatal(err)
	}
	if !c.Debrief.ReplaceEditing || c.Debrief.ReplaceRow != 1 {
		t.Fatal("Backspace did not use visible keyboard-selected row")
	}
	if err = c.Handle(Actions{Inside: true, X: actual[0], Y: actual[1], Cancel: true}); err != nil {
		t.Fatal(err)
	}
	check(1, centre(1))
	if err = c.Handle(Actions{Inside: true, X: actual[0], Y: actual[1], Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "feedback" || c.pointerCue != nil {
		t.Fatal("REPLACE pointer owner leaked to successor")
	}
}

func TestReplaceActualChoicePageOrder(t *testing.T) {
	c := replacementDisplay(t)
	p := c.Debrief.Pending.Proposal
	p.Commission = true
	p.RefusedResult = &gamestate.SortieResultDetails{CrewPro: false}
	accepted := true
	c.Debrief.Pending.CommissionChoice = &accepted
	c.enterDebriefPage("jump")
	c.Debrief.Tick = 400
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "replace" {
		t.Fatal("accepted oneactor commission omitted REPLACE")
	}
	accepted = false
	c.enterDebriefPage("jump")
	c.Debrief.Tick = 400
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "feedback" {
		t.Fatal("refused oneactor commission entered replacement")
	}
	p.RefusedResult.CrewPro = true
	c.enterDebriefPage("jump")
	c.Debrief.Tick = 400
	if err := c.handleDebrief(Actions{Enter: true}); err != nil {
		t.Fatal(err)
	}
	if c.Debrief.Page != "crew" {
		t.Fatal("refused fiveactor offer suppressed CREWPRO")
	}
}
