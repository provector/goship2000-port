package menus

import (
	"bytes"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// The original chooser's second flag08 FMHD is a two-component Squad. Its
// type1 detail symbol is a three-glyph group mark, but the fixed overview's
// C504 caller draws exactly one S. Neither symbol count changes the mission.
func TestTroopDropHasOneOverviewSiteAndSourceDetailGroup(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	for _, role := range []string{"primary", "secondary"} {
		for _, kind := range []int{5, 7} {
			opts, err := gamestate.AuthoredOptions(c.editorData(), e.Document, role, kind)
			if err != nil || len(opts) != 1 || opts[0].Cost != map[int]int{5: 2, 7: 1}[kind] {
				t.Fatal(role, opts, err)
			}
			o := opts[0]
			if kind == 5 && o.Name != "Squad" {
				t.Fatal("source second flag08", o)
			}
			for _, formation := range []bool{false, true} {
				actual, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, role, kind, 0, formation)
				wantJSON, _ := json.Marshal(opts)
				actualJSON, _ := json.Marshal(actual)
				if err != nil || !bytes.Equal(wantJSON, actualJSON) {
					t.Fatal("tactical chooser must ignore ordinary Single/Group", formation, err)
				}
			}
			p := gamestate.AuthoredPlacement{Role: role, Kind: kind, Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile, X: 108, Y: 81}
			rows, err := gamestate.AuthoredComponents(c.editorData(), e.Document, p)
			if err != nil || len(rows) != map[int]int{5: 2, 7: 1}[kind] {
				t.Fatal("source tactical component span differs", rows, err)
			}
			e.Document.Placements = []gamestate.AuthoredPlacement{p}
			e.Role, e.Category = "", 0
			e.View.Detail = true
			e.View.CameraX, e.View.CameraY = -170, 1017
			e.View.Bounds = [4]int{170, 65, 147, 123}
			e.FilterInitialized = true
			e.Filter = 0x3ff
			before, _ := json.Marshal(e.Document)
			index := 12
			if role == "secondary" {
				index = 13
			}
			for _, overview := range []bool{true, false} {
				e.paintOverview = overview
				actual, want := make([]byte, 64000), make([]byte, 64000)
				c.drawEditorPlacedSymbols(actual, overview)
				x, y := c.editorScreen(p.X, p.Y)
				c.editorIconRow(want, index, x, y, false, 0)
				if !overview && kind == 5 {
					c.editorIconRow(want, index, x-4, y+5, false, 0)
					c.editorIconRow(want, index, x+4, y+5, false, 0)
				}
				if overview && kind == 5 {
					//Retain the old erroneous shared painter as a discriminating
					//negative control; its extra flanks must differ from source.
					wrong := append([]byte(nil), want...)
					c.editorFormationIcon(wrong, p, index, x, y, 0)
					if bytes.Equal(wrong, want) {
						t.Fatal("old overview bug control did not draw extra sites")
					}
				}
				if !bytes.Equal(actual, want) {
					t.Fatalf("%s overview=%v: source-specific marker owner differs", role, overview)
				}
			}
			after, _ := json.Marshal(e.Document)
			if !bytes.Equal(before, after) {
				t.Fatal("painting changed Squad/document/RNG")
			}
		}
	}
}
