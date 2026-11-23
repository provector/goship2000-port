package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"strings"
	"testing"
)

// Controller input receipts complement, and do not claim to replace, OS input.
// Source validation1abf2678 warns for missing FARP; it is not an invalid mission.
func TestSuccessiveMapNamesOptionalFARPAndFrozenReadback(t *testing.T) {
	c := authoredUIController(t)
	before, _ := json.Marshal(c.Store.State())
	for cycle := 0; cycle < 3; cycle++ {
		if c.Screen != ScreenMissionFiles {
			if err := c.openMissionFiles(); err != nil {
				t.Fatal(err)
			}
		}
		editorKey(t, c, "n")
		if c.Editor == nil || c.Editor.Stage != "world" {
			t.Fatal("NEW", cycle)
		}
		name := c.Editor.Worlds[cycle%len(c.Editor.Worlds)]
		preview := c.editorAssets().Previews[editorMapKey(name)]
		editorClick(t, c, preview.X+2, preview.Y+2)
		editorKey(t, c, "b")
		r := c.Editor.popupArea(0, len(c.Editor.Options))
		editorClick(t, c, r[0]+2, r[1]+2)
		editorClick(t, c, 87, 59)
		for _, v := range []struct {
			k          string
			kind, x, y int
		}{{"p", 7, 66, 70}, {"s", 5, 87, 70}} {
			editorKey(t, c, v.k)
			r = c.Editor.popupArea(v.kind, 8)
			editorClick(t, c, r[0]+2, r[1]+2)
			r = c.Editor.popupArea(0, len(c.Editor.Options))
			editorClick(t, c, r[0]+2, r[1]+2)
			editorClick(t, c, v.x, v.y)
		}
		for _, p := range c.Editor.Document.Placements {
			if p.Role == "FARP" {
				t.Fatal("fabricated FARP")
			}
		}
		editorKey(t, c, "x")
		editorKey(t, c, "s")
		if c.Editor.Stage != "title" {
			t.Fatal("valid optional-FARP SAVE did not reach text", cycle, c.Editor.Stage, c.Editor.Status)
		}
		// Cancel/reopen the name modal without discarding the draft.
		id := c.Editor.Document.ID
		editorAction(t, c, Actions{Cancel: true})
		editorKey(t, c, "s")
		if c.Editor.Document.ID != id || c.Editor.Stage != "title" {
			t.Fatal("cancel changed draft")
		}
		want := fmt.Sprintf("Map %c mission", 'A'+cycle)
		var latch inputLatch
		for _, ch := range want + "Z" {
			editorAction(t, c, latch.Read(Buttons{Text: []rune{ch}, AnyKeyPressed: true}))
		}
		editorAction(t, c, latch.Read(Buttons{Backspace: true}))
		editorAction(t, c, latch.Read(Buttons{}))
		editorAction(t, c, latch.Read(Buttons{Enter: true}))
		editorAction(t, c, latch.Read(Buttons{}))
		if c.Editor != nil || c.Screen != ScreenMissionFiles {
			t.Fatal("SAVE", cycle, c.Editor)
		}
		d, rev, err := c.Files.Store.Load(id)
		if err != nil {
			t.Fatal(err)
		}
		if d.Title != want || d.World != name || d.Compiled == nil {
			t.Fatal("saved identity", d.Title, d.World, name)
		}
		if _, ok := d.Compiled.Objectives["FARP"]; ok {
			t.Fatal("absent FARP acquired a compiled objective")
		}
		if d.Compiled.Targets.Records[1][0] != 0x20 {
			t.Fatal("missing-FARP TARGET must retain constructor default")
		}
		frozen, _ := json.Marshal(d.Compiled)
		if err = c.editMissionDocument(d, rev); err != nil {
			t.Fatal(err)
		}
		editorKey(t, c, "x")
		editorKey(t, c, "e")
		editorKey(t, c, "x")
		editorKey(t, c, "s")
		read, _, err := c.Files.Store.Load(id)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(read.Compiled)
		if !bytes.Equal(frozen, after) {
			t.Fatal("unchanged resave changed realization")
		}
	}
	after, _ := json.Marshal(c.Store.State())
	if !bytes.Equal(before, after) {
		t.Fatal("authoring changed career")
	}
}

func TestBriefingSupportUsesFrozenFormationOnS2Only(t *testing.T) {
	c := authoredUIController(t)
	d := authorThroughUI(t, c)
	for _, mode := range []int{0, 1, 2} {
		q, err := gamestate.CloneAuthored(d)
		if err != nil {
			t.Fatal(err)
		}
		if mode > 0 {
			opts, err := gamestate.AuthoredSupportOptions(c.editorData(), q)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			w, _ := gamestate.AuthoredWorld(c.editorData(), q.World)
			for _, o := range opts {
				if o.Template < 0 {
					continue
				}
				kind := w.OBJL[o.Template*32+10]
				if (mode == 1 && kind == 0x19) || (mode == 2 && kind == 0x11) {
					if err = gamestate.SetAuthoredSupport(c.editorData(), q, o); err != nil {
						t.Fatal(err)
					}
					found = true
					break
				}
			}
			if !found {
				t.Fatal("Support fixture unavailable", mode)
			}
		}
		p := c.Store.State().SelectedPilot()
		m, err := gamestate.CompileAuthoredMission(c.editorData(), q, gamestate.MissionInputs{MissionType: gamestate.MissionSimulated, Theatre: p.Theatre, Rank: p.Rank, PilotName: p.Name, Seed: int64(q.Seed)})
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewBriefingView(c.Office.Assets.Missions, m, p, 1)
		if err != nil {
			t.Fatal(err)
		}
		intel := v.IntelligenceText()
		orders := v.OrdersText()
		if strings.Contains(orders, "will be available") {
			t.Fatal("Support belongs to S-2, not unconditional Orders")
		}
		if got := strings.Contains(intel, "will be available for this mission"); got != (mode != 0) {
			t.Fatal(mode, intel)
		}
		if mode != 0 && !strings.Contains(intel, v.friendlySupportTitle()) {
			t.Fatal("actual frozen Support title lost")
		}
	}
}
