package menus

import (
	"bytes"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestEditorSpecialCompletionCancelAndBothToggleOwners(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	editorKey(t, c, "b")
	r := e.popupArea(0, len(e.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	if !e.Held {
		t.Fatal("base command not armed")
	}
	editorClick(t, c, 87, 59)
	if e.Held || e.Role != "" || e.Category != 0 || e.Shadow != nil {
		t.Fatal("source completed special retained AREA", e)
	}
	opts, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 0, false)
	if err != nil || len(opts) == 0 {
		t.Fatal(err)
	}
	if err = c.editorArm("defender", 0, opts[0].Category); err != nil {
		t.Fatal(err)
	}
	r = e.popupArea(0, len(e.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	if !e.Held || e.Role != "defender" {
		t.Fatal("ordinary not armed", e.Status)
	}
	editorAction(t, c, Actions{Cancel: true})
	if e.Held || e.Role != "" || e.Category != 0 {
		t.Fatal("cancel retained category paint")
	}
	start := e.Formation
	c.editorControlCommand(18)
	if e.Formation == start {
		t.Fatal("Single/Group18 failedtoggle")
	}
	c.editorControlCommand(19)
	if e.Formation != start {
		t.Fatal("Single/Group19 isnot sharedtoggle")
	}
	before := gamestate.AuthoredRevision(e.Document)
	for i := 0; i < 20; i++ {
		editorAction(t, c, Actions{Skip: true})
	}
	if before != gamestate.AuthoredRevision(e.Document) {
		t.Fatal("view toggle wrote document/RNG")
	}
	for y := 189; y <= 199; y++ {
		e.CursorX, e.CursorY = 290, y
		if c.editorPointerStyle() != 6 {
			t.Fatal("EXIT cursor priority", y)
		}
	}
}
func TestRosterReplacementDetachedAndSafeEditSeparate(t *testing.T) {
	c := sourceFrontEnd(t, true)
	state := c.Store.State()
	p := state.Slots[0]
	p.Rank = gamestate.RankCaptain
	p.Statistics.CareerScore = 18333
	p.Squadron = "Decorated unit"
	p.LegacyRaw[0x4e] = 79
	second, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	second.Name = "Other"
	second.Squadron = "Other unit"
	state.Slots[1] = second
	if err = c.Store.Commit(state); err != nil {
		t.Fatal(err)
	}
	c.RefreshOffice()
	officeClick(t, c, 184, 70)
	before := c.Store.State()
	old := before.Slots[0]
	other, _ := json.Marshal(before.Slots[1])
	editorAction(t, c, Actions{Selector2: true})
	if c.Draft == nil || !c.Draft.Replacement || c.Draft.Pilot.ID == old.ID || c.Draft.Pilot.Rank != gamestate.RankWOC || c.Draft.Pilot.Statistics.CareerScore != 0 || c.Draft.Pilot.Squadron != "" || len(c.Draft.Pilot.SquadronPilots) != 0 || c.Draft.Pilot.LegacyRaw[0x4e] != 79 {
		t.Fatal("fresh source clear/identity", c.Draft)
	}
	editorAction(t, c, Actions{Text: []rune("Fresh"), Enter: true})
	editorAction(t, c, Actions{Enter: true})
	editorAction(t, c, Actions{Text: []rune("Fresh unit"), Enter: true})
	if c.Draft != nil || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("replacement published beforePOST")
	}
	replacement := c.rosterState().Slots[0]
	editorAction(t, c, Actions{Cancel: true})
	if !reflect.DeepEqual(before, c.Store.State()) || c.rosterWorking != nil {
		t.Fatal("outerESC published/keptdraft")
	}
	officeClick(t, c, 184, 70)
	editorAction(t, c, Actions{Selector2: true})
	editorAction(t, c, Actions{Text: []rune("Fresh"), Enter: true})
	editorAction(t, c, Actions{Enter: true})
	editorAction(t, c, Actions{Text: []rune("Fresh unit"), Enter: true})
	replacement = c.rosterState().Slots[0]
	editorAction(t, c, Actions{Selector1: true})
	after := c.Store.State()
	if after.SelectedPilotID != replacement.ID || after.Slots[0].ID != replacement.ID || after.Slots[0].ID == old.ID || after.Slots[0].Statistics.CareerScore != 0 || after.Slots[0].Rank != gamestate.RankWOC {
		t.Fatal("replacementPOST", after)
	}
	gotOther, _ := json.Marshal(after.Slots[1])
	if !bytes.Equal(other, gotOther) {
		t.Fatal("otherpilot changed")
	}
	officeClick(t, c, 184, 70)
	editorAction(t, c, Actions{Text: []rune("e")})
	if c.Draft == nil || c.Draft.Replacement || c.Draft.Pilot.ID != replacement.ID {
		t.Fatal("safeEdit not separate")
	}
	editorAction(t, c, Actions{Cancel: true})
}
func TestEditorSupportChooserNoneAndAirRetained(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	before := c.Store.State()
	editorKey(t, c, "u")
	if e.Stage != "support" || len(e.Options) != 6 {
		t.Fatal(e.Stage, e.Options)
	}
	battery := -1
	air := -1
	w, _ := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	for i, o := range e.Options {
		if i == 0 {
			continue
		}
		if w.OBJL[o.Template*32+10] == 0x19 {
			battery = i
		} else {
			air = i
		}
	}
	r := e.popupArea(battery, len(e.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	if e.Document.Support == nil || e.Document.Support.Kind != 0x19 || e.Held {
		t.Fatal("battery chooser", e.Status)
	}
	editorKey(t, c, "u")
	editorAction(t, c, Actions{Cancel: true})
	if e.Document.Support == nil || e.Document.Support.Kind != 0x19 || e.Document.SupportMemory != nil {
		t.Fatal("Esc changed the accepted Support choice")
	}
	editorKey(t, c, "u")
	r = e.popupArea(air, len(e.Options))
	editorClick(t, c, r[0]+2, r[1]+2)
	if e.Document.Support == nil || e.Document.Support.Kind != 0x11 {
		t.Fatal("air authoring dropped", e.Status)
	}
	if gamestate.AuthoredExecutionReady(c.editorData(), e.Document) == nil {
		t.Fatal("unsupported activeair accepted")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("chooser wrote career")
	}
}

func TestFreshSupportEscAndExplicitNoneHaveOwnedCosts(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	baseBudget, _ := gamestate.AuthoredBudget(c.editorData(), e.Document)
	editorKey(t, c, "u")
	battery := -1
	for i, o := range e.Options {
		if o.Cost == 3 {
			battery = i
			break
		}
	}
	if battery < 0 {
		t.Fatal("source battery missing")
	}
	area := e.popupArea(battery, len(e.Options))
	editorClick(t, c, area[0]+2, area[1]+2)
	accepted := gamestate.AuthoredRevision(e.Document)
	editorKey(t, c, "u")
	editorAction(t, c, Actions{Cancel: true})
	budget, _ := gamestate.AuthoredBudget(c.editorData(), e.Document)
	if e.Document.Support == nil || e.Document.SupportMemory != nil || e.supportCancellationGuarded() || budget != baseBudget-3 || gamestate.AuthoredRevision(e.Document) != accepted {
		t.Fatal("Esc leaked or discarded Support", budget)
	}
	editorKey(t, c, "u")
	area = e.popupArea(0, len(e.Options))
	editorClick(t, c, area[0]+2, area[1]+2)
	budget, _ = gamestate.AuthoredBudget(c.editorData(), e.Document)
	if e.Document.Support != nil || e.Document.SupportMemory != nil || budget != baseBudget || e.supportCancellationGuarded() {
		t.Fatal("None did not remove only owned Support", budget)
	}
}
