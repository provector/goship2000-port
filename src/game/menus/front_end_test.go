package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPlayerSelectorsAreEdgesAndDoNotCarryThrough(t *testing.T) {
	latch := inputLatch{}
	b := Buttons{Space: true, Enter: true, Text: []rune{' '}, X: 184, Y: 70, Inside: true}
	a := latch.Read(b)
	if !a.Selector1 || !a.Selector2 || !a.Enter || string(a.Text) != " " {
		t.Fatal(a)
	}
	a = latch.Read(b)
	if a.Selector1 || a.Selector2 || a.Enter {
		t.Fatal("held input repeated", a)
	}
	latch.Read(Buttons{})
	if !latch.Read(Buttons{RightMouse: true}).Selector2 {
		t.Fatal("right button not Selector2")
	}
	if latch.Read(Buttons{RightMouse: true}).Selector2 {
		t.Fatal("held mouse repeated")
	}
}

func TestFrontEndRankAndOpaqueEligibility(t *testing.T) {
	p, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	got, err := missionChoices(p)
	if err != nil || !reflect.DeepEqual(got, []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionSingle}) {
		t.Fatal(got, err)
	}
	p.Rank = gamestate.RankCaptain
	got, err = missionChoices(p)
	if err != nil || !reflect.DeepEqual(got, []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionSingle, gamestate.MissionFlight, gamestate.MissionCampaign, gamestate.MissionSimulated}) {
		t.Fatal(got, err)
	}
	p.LegacyRaw[0x4f] = 1
	got, err = missionChoices(p)
	if err != nil || !reflect.DeepEqual(got, []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionCampaign}) {
		t.Fatal(got, err)
	}
	p.MissionType = gamestate.MissionCampaign
	if locked, err := theatreLocked(p); err != nil || !locked {
		t.Fatal(locked, err)
	}
	p.LegacyRaw = nil
	if _, err := missionChoices(p); err == nil {
		t.Fatal("unknown eligibility silently treated as zero")
	}
	if _, err := theatreLocked(p); err == nil {
		t.Fatal("unknown campaign restriction silently treated as zero")
	}
}

func sourceFrontEnd(t *testing.T, active bool) *FrontEnd {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_MENUS")
	if dir == "" {
		t.Skip("set GS2000_TEST_MENUS for prepared front-end checks")
	}
	assets, err := assetdata.LoadOfficeAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	assets.Menus, err = assetdata.LoadMenuAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := gamestate.OpenSaveStore(filepath.Join(root, "saves"), filepath.Join(root, "original-not-present"))
	if err != nil {
		t.Fatal(err)
	}
	if active {
		p, err := gamestate.NewPilot()
		if err != nil {
			t.Fatal(err)
		}
		p.Name = "Test Pilot Name"
		p.Squadron = "Test Squadron Name"
		p.Emblem = 1
		state := store.State()
		state.Slots[0] = p
		state.SelectedPilotID = p.ID
		if err = store.Commit(state); err != nil {
			t.Fatal(err)
		}
	}
	c, err := NewFrontEnd(assets, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.AbandonFilmRecording)
	return c
}

func act(t *testing.T, c *FrontEnd, a Actions) {
	t.Helper()
	// These semantic tests specify absolute targets, unlike the dedicated delta-pointer tests.
	if c.Screen == ScreenMission {
		a.PointerReset = true
	}
	if err := c.Handle(a); err != nil {
		t.Fatal(err)
	}
}

func officeClick(t *testing.T, c *FrontEnd, x, y int) {
	t.Helper()
	act(t, c, Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true})
}

func finishFolder(t *testing.T, c *FrontEnd) {
	t.Helper()
	for i := 0; i < 1000 && c.Screen == ScreenFolder; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenMission {
		t.Fatal(c.Screen)
	}
}

func TestFrontEndGenuineEmptyGuardAndDraftCommit(t *testing.T) {
	c := sourceFrontEnd(t, false)
	if p := c.Store.State().SelectedPilot(); p != nil {
		t.Fatal("research pilot created on normal startup")
	}
	officeClick(t, c, 100, 170)
	if c.Screen != ScreenOffice || c.Office.PointerX != 184 || c.Office.PointerY != 70 {
		t.Fatal("active guard/cue failed", c.Screen, c.Office.PointerX, c.Office.PointerY)
	}
	// Pointer remains at the roster cue until real movement, without OS injection.
	act(t, c, Actions{X: 100, Y: 170, Inside: true})
	if c.Office.HoverKey != 1 {
		t.Fatal("roster cue lost before pointer movement")
	}
	act(t, c, Actions{X: 100, Y: 170, Inside: true, Selector1: true})
	if c.Screen != ScreenRoster {
		t.Fatal(c.Screen)
	}
	before := c.Store.State()
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector2: true, PointerPressed: true})
	if c.Draft == nil || c.Draft.Stage != 0 {
		t.Fatal("editor not opened")
	}
	// Space is data here, although it also denotes Selector1 outside the editor.
	act(t, c, Actions{Text: []rune("Test Pilot"), Selector1: true})
	act(t, c, Actions{Text: []rune(" Name"), Enter: true, Selector2: true})
	if c.Draft.Stage != 1 || c.Draft.Pilot.Name != "Test Pilot Name" {
		t.Fatal(c.Draft)
	}
	for i := 0; i < 18; i++ {
		act(t, c, Actions{Right: true})
	}
	if c.Draft.Pilot.Emblem != 0 {
		t.Fatal("18 emblems did not wrap")
	}
	act(t, c, Actions{Right: true})
	act(t, c, Actions{Enter: true, Selector2: true})
	act(t, c, Actions{Enter: true, Selector2: true})
	if c.Draft == nil || c.Store.State().Revision != before.Revision {
		t.Fatal("empty unit committed")
	}
	act(t, c, Actions{Text: []rune("Test Squadron Name"), Enter: true, Selector2: true})
	state := c.rosterState()
	if c.Store.State().Slots[0] != nil {
		t.Fatal("replacement published beforePOST")
	}
	if c.Draft != nil || state.Slots[0] == nil || state.Slots[0].Name != "Test Pilot Name" || state.Slots[0].Squadron != "Test Squadron Name" || state.Slots[0].Emblem != 1 || state.SelectedPilotID != "" {
		t.Fatal("creation/posting distinction failed", state)
	}
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector1: true, PointerPressed: true})
	if c.Screen != ScreenOffice || c.Office.State.PilotName != "Test Pilot Name" || !c.Office.State.Active {
		t.Fatal("posting/office refresh failed")
	}
}

func TestFrontEndDraftCancelAndSafeExistingEdit(t *testing.T) {
	c := sourceFrontEnd(t, true)
	before := c.Store.State()
	officeClick(t, c, 184, 70)
	act(t, c, Actions{Text: []rune("e")})
	c.Draft.Pilot.Rank = gamestate.RankCaptain // Draft-only mutation never reaches live state.
	act(t, c, Actions{Text: []rune("XYZ")})
	act(t, c, Actions{Cancel: true})
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("cancel mutated save")
	}
	act(t, c, Actions{Text: []rune("e")})
	c.Draft.Pilot.Name = "Changed Name"
	act(t, c, Actions{Enter: true})
	act(t, c, Actions{Enter: true})
	act(t, c, Actions{Enter: true})
	after := c.Store.State()
	if after.Slots[0].Name != "Changed Name" || after.Slots[0].Rank != before.Slots[0].Rank || after.Slots[0].ID != before.Slots[0].ID || !bytes.Equal(after.Slots[0].LegacyRaw, before.Slots[0].LegacyRaw) {
		t.Fatal("safe editor reset progress or identity")
	}
}

func TestFrontEndTheatreAndMissionCommitCancelRefresh(t *testing.T) {
	c := sourceFrontEnd(t, true)
	before := c.Store.State()
	oldScene := append([]byte(nil), c.Office.Scene...)
	officeClick(t, c, 270, 45)
	if c.Screen != ScreenTheatre || c.Highlight != 1 {
		t.Fatal(c.Screen, c.Highlight)
	}
	box := c.Menus.Theatres[3].Rectangle
	act(t, c, Actions{X: box[0] + 2, Y: box[1] + 2, Inside: true})
	act(t, c, Actions{Cancel: true})
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("theatre cancel changed state")
	}
	officeClick(t, c, 270, 45)
	act(t, c, Actions{X: box[0] + 2, Y: box[1] + 2, Inside: true, Selector1: true, PointerPressed: true})
	if c.Screen != ScreenOffice || c.Office.State.Theatre != 3 || bytes.Equal(oldScene, c.Office.Scene) {
		t.Fatal("theatre commit/refresh failed")
	}
	before = c.Store.State()
	officeClick(t, c, 100, 170)
	finishFolder(t, c)
	if len(c.TransitionGroups) == 0 || c.TransitionGroups[len(c.TransitionGroups)-1] != 31 {
		t.Fatal("folder transition incomplete")
	}
	for i := 1; i < len(c.TransitionGroups); i++ {
		if c.TransitionGroups[i] != c.TransitionGroups[i-1]+1 {
			t.Fatal("transition out of order")
		}
	}
	act(t, c, Actions{X: 100, Y: 92, Inside: true})
	act(t, c, Actions{Cancel: true})
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("mission cancel committed highlight")
	}
	officeClick(t, c, 100, 170)
	finishFolder(t, c)
	act(t, c, Actions{X: 100, Y: 92, Inside: true, Selector1: true, PointerPressed: true})
	if c.Screen != ScreenOffice || c.Store.State().SelectedPilot().MissionType != gamestate.MissionSingle || c.Office.State.MissionType != 1 {
		t.Fatal("mission commit refresh failed")
	}
}

func TestFrontEndRecoveryCancelRestoreAndQuit(t *testing.T) {
	c := sourceFrontEnd(t, true)
	// This explicit fixture represents a mission boundary, not a menu undo point.
	if err := c.Store.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	next := c.Store.State()
	next.SelectedPilot().Theatre = gamestate.TheatrePhilippines
	if err := c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	before := c.Store.State()
	officeClick(t, c, 300, 140)
	if c.Screen != ScreenRecovery {
		t.Fatal(c.Screen)
	}
	act(t, c, Actions{Cancel: true})
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("recovery cancel wrote")
	}
	officeClick(t, c, 300, 140)
	act(t, c, Actions{Selector1: true})
	if c.Screen != ScreenOffice || c.Office.State.Theatre != 1 {
		t.Fatal("restore refresh failed")
	}
	act(t, c, Actions{Cancel: true})
	if c.ExitRequested || c.Screen != ScreenOffice {
		t.Fatal("office Escape quit")
	}
	act(t, c, Actions{Quit: true})
	if c.Screen != ScreenQuit {
		t.Fatal(c.Screen)
	}
	act(t, c, Actions{Text: []rune{'n'}})
	if c.Screen != ScreenOffice || c.ExitRequested {
		t.Fatal("N did not decline")
	}
	act(t, c, Actions{Close: true})
	if c.Screen != ScreenQuit {
		t.Fatal("close did not request confirmation")
	}
	act(t, c, Actions{Text: []rune{'Y'}})
	if !c.ExitRequested {
		t.Fatal("Y did not confirm")
	}
}

func TestFrontEndUnimplementedOfficeActionIsHarmless(t *testing.T) {
	c := sourceFrontEnd(t, true)
	before := c.Store.State()
	// Cabinet8 is implemented by Step O and covered by Film Library checks.
	officeClick(t, c, 240, 70)
	if c.Screen != ScreenOffice || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("unimplemented destination activated")
	}
}

func TestFrontEndTextLimitsAndLeftDeletion(t *testing.T) {
	c := sourceFrontEnd(t, false)
	officeClick(t, c, 184, 70)
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector2: true, PointerPressed: true})
	act(t, c, Actions{Text: []rune("  Abc ")})
	if c.Draft.Pilot.Name != "Abc " {
		t.Fatal(c.Draft.Pilot.Name)
	}
	act(t, c, Actions{Left: true})
	if c.Draft.Pilot.Name != "Abc" {
		t.Fatal("left did not delete final byte")
	}
	act(t, c, Actions{Text: []rune{'é', 1, 127}})
	if c.Draft.Pilot.Name != "Abc\x7f" {
		t.Fatal("acceptance/case rules", c.Draft.Pilot.Name)
	}
	act(t, c, Actions{Backspace: true})
	act(t, c, Actions{Text: []rune("iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii")})
	if len(c.Draft.Pilot.Name) > 35 || c.Menus.Fonts[5].Measure([]byte(c.Draft.Pilot.Name)) > 200 {
		t.Fatal("text bounds exceeded")
	}
}

func TestFrontEndHeldOpeningEnterDoesNotFinishEditor(t *testing.T) {
	c := sourceFrontEnd(t, true)
	officeClick(t, c, 184, 70)
	latch := inputLatch{}
	first := Buttons{Enter: true, Inside: true, X: 30, Y: 90}
	act(t, c, latch.Read(first))
	if c.Draft == nil {
		t.Fatal("Enter did not map to Selector2")
	}
	act(t, c, latch.Read(first))
	if c.Draft.Stage != 0 {
		t.Fatal("opening Enter carried into field completion")
	}
}

func TestFrontEndPreparedPreviewExportWhenRequested(t *testing.T) {
	output := os.Getenv("GS2000_MENU_PREVIEW_OUTPUT")
	if output == "" {
		t.Skip("explicit new preview directory required")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("preview directory must not exist")
	}
	c := sourceFrontEnd(t, false)
	capture := func(name string) {
		t.Helper()
		if err := WriteFrontEndFrame(filepath.Join(output, name+".png"), c, false); err != nil {
			t.Fatal(err)
		}
	}
	capture("empty-office")
	officeClick(t, c, 184, 70)
	capture("empty-roster")
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector2: true, PointerPressed: true})
	act(t, c, Actions{Text: []rune("Test Pilot Name")})
	capture("editor-name")
	act(t, c, Actions{Enter: true})
	act(t, c, Actions{Right: true})
	capture("editor-emblem01")
	act(t, c, Actions{Enter: true})
	act(t, c, Actions{Text: []rune("Test Squadron Name")})
	capture("editor-unit")
	act(t, c, Actions{Enter: true})
	capture("created-not-posted")
	act(t, c, Actions{X: 30, Y: 90, Inside: true, Selector1: true, PointerPressed: true})
	capture("posted-WOC-office")
	if err := c.Store.PrepareMissionSnapshot(); err != nil {
		t.Fatal(err)
	}
	officeClick(t, c, 270, 45)
	capture("theatre")
	act(t, c, Actions{Cancel: true})
	officeClick(t, c, 100, 170)
	finishFolder(t, c)
	capture("WOC-mission-choices")
	act(t, c, Actions{Cancel: true})
	state := c.Store.State()
	state.SelectedPilot().Rank = gamestate.RankCaptain
	if err := c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	if err := c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	capture("captain-office")
	officeClick(t, c, 100, 170)
	finishFolder(t, c)
	capture("captain-mission-choices")
	act(t, c, Actions{Cancel: true})
	officeClick(t, c, 300, 140)
	capture("recovery-confirmation")
	act(t, c, Actions{Cancel: true})
	act(t, c, Actions{Quit: true})
	capture("quit-confirmation")
}
