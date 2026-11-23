package menus

import (
	"bytes"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func placedFARP(c *FrontEnd) *gamestate.AuthoredPlacement {
	if c.Editor == nil {
		return nil
	}
	for _, p := range c.Editor.Document.Placements {
		if p.Role == "FARP" {
			q := p
			return &q
		}
	}
	return nil
}
func TestFARPControlLiftCancelMoveAndSaveReadback(t *testing.T) {
	c := controlsEditor(t)
	d, _ := gamestate.CloneAuthored(c.Editor.Document)
	d.Title = "FARP lifecycle"
	if err := c.openMissionFiles(); err != nil {
		t.Fatal(err)
	}
	if err := c.editMissionDocument(d, ""); err != nil {
		t.Fatal(err)
	}
	beforeCareer, _ := json.Marshal(c.Store.State())
	editorKey(t, c, "f")
	if c.Editor.Stage != "edit" || !c.Editor.Held || c.Editor.Role != "FARP" {
		t.Fatal("F must arm first source FARP directly", c.Editor.Stage, c.Editor.Status)
	}
	editorClick(t, c, 105, 70)
	old := placedFARP(c)
	if old == nil || c.Editor.Held {
		t.Fatal("FARP placement", c.Editor.Status)
	}
	remaining, _ := gamestate.AuthoredBudget(c.editorData(), c.Editor.Document)
	editorKey(t, c, "f")
	if placedFARP(c) != nil || !c.Editor.Held || c.Editor.LiftedFARP == nil || !c.Editor.Dirty {
		t.Fatal("F did not lift immediately", c.Editor.Status)
	}
	refunded, _ := gamestate.AuthoredBudget(c.editorData(), c.Editor.Document)
	rows, _ := gamestate.AuthoredComponents(c.editorData(), c.Editor.Document, *old)
	if refunded != remaining+len(rows) {
		t.Fatal("FARP cost refund", remaining, refunded)
	}
	editorAction(t, c, Actions{Cancel: true})
	if placedFARP(c) != nil || c.Editor.Held || c.Editor.LiftedFARP != nil {
		t.Fatal("Esc restored removed FARP")
	}
	editorKey(t, c, "f")
	editorClick(t, c, 100, 90)
	placed := placedFARP(c)
	if placed == nil {
		t.Fatal(c.Editor.Status)
	}
	//The source marker pickup enters the very same205C transition.
	x, y := c.editorScreen(placed.X, placed.Y)
	editorAction(t, c, Actions{X: x, Y: y, Inside: true, PointerPressed: true, Selector2: true})
	if placedFARP(c) != nil || !c.Editor.Held || c.Editor.LiftedFARP == nil {
		t.Fatal("selector2 FARP pickup", c.Editor.Status)
	}
	editorClick(t, c, 115, 100)
	moved := placedFARP(c)
	if moved == nil || moved.ID != placed.ID || moved.X == placed.X && moved.Y == placed.Y || c.Editor.LiftedFARP != nil {
		t.Fatal("moved descriptor lost identity", placed, moved, c.Editor.Status)
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil || c.Screen != ScreenMissionFiles {
		t.Fatal("placed FARP SAVE", c.Editor)
	}
	saved, rev, err := c.Files.Store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.Compiled.Objectives["FARP"]; !ok {
		t.Fatal("saved FARP missing")
	}
	if err = c.editMissionDocument(saved, rev); err != nil {
		t.Fatal(err)
	}
	editorKey(t, c, "f")
	editorAction(t, c, Actions{Cancel: true})
	if c.Editor.Document.Compiled != nil || placedFARP(c) != nil {
		t.Fatal("lift did not invalidate realization")
	}
	editorKey(t, c, "x")
	editorKey(t, c, "s")
	if c.Editor != nil {
		t.Fatal("absent FARP SAVE", c.Editor.Status)
	}
	read, _, err := c.Files.Store.Load(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := read.Compiled.Objectives["FARP"]; ok || read.Compiled.Targets.Records[1][0] != 0x20 {
		t.Fatal("phantom FARP in saved/BEGIN state")
	}
	frozen, _ := json.Marshal(read.Compiled)
	p := c.Store.State().SelectedPilot()
	m, err := gamestate.CompileAuthoredMission(c.editorData(), read, gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: int64(read.Seed)})
	if err != nil {
		t.Fatal("no FARP BEGIN", err)
	}
	_, hasFARP := m.Objectives["FARP"]
	if hasFARP {
		t.Fatal("no FARP BEGIN", err)
	}
	final, _ := json.Marshal(read.Compiled)
	if !bytes.Equal(frozen, final) {
		t.Fatal("unchanged BEGIN recompiled")
	}
	afterCareer, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(beforeCareer, afterCareer) {
		t.Fatal("FARP editing changed career")
	}
}
