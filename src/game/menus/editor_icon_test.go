package menus

import (
	"bytes"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestEditorOrdinaryIconSourceRowsAndPixels(t *testing.T) {
	c := gulfEditor(t)
	w, err := gamestate.AuthoredWorld(c.editorData(), "GULF5.WRL")
	if err != nil {
		t.Fatal(err)
	}
	//Independent retained GULF5 operands: Bunker322 is armed; formation167
	//starts at323 with weapon153, but its later324..326 components are unarmed.
	if !bytes.Equal(w.FRMD[322*8:322*8+4], []byte{55, 22, 1, 147}) || !bytes.Equal(w.FRMD[323*8:323*8+4], []byte{137, 44, 1, 153}) || !bytes.Equal(w.FRMD[324*8:324*8+4], []byte{255, 87, 1, 0}) || w.FRMD[326*8+3] != 0 {
		t.Fatal("source component/weapon operands changed")
	}
	if c.editorAssets().IconRects[9] != ([4]int{102, 0, 5, 4}) || c.editorAssets().IconAnchors[9] != ([2]int{2, 1}) {
		t.Fatal("DS2b44 source structure crop")
	}
	beforeFRMD := bytes.Clone(w.FRMD)
	c.Editor.All, c.Editor.Paths, c.Editor.Weapons = false, false, false
	//2114 is the detail-atlas caller, not the tiny overview540A caller.
	c.Editor.View = newEditorMapView(3)
	c.Editor.View.Detail = true
	c.Editor.View.CameraX, c.Editor.View.CameraY = 640, 401
	for _, q := range []struct {
		label            string
		fi, ci, selected int
		profile          string
		row              int
	}{
		{"armed", 166, 322, 0, gamestate.AuthoredSelectionProfile, 0},
		{"later-unarmed", 167, 323, 0, gamestate.AuthoredSelectionProfile, 4},
		{"selected-armed", 166, 322, 10, gamestate.AuthoredSelectionProfile, 16},
		{"selected-later-unarmed", 167, 323, 10, gamestate.AuthoredSelectionProfile, 20},
		{"other-selected", 167, 323, 9, gamestate.AuthoredSelectionProfile, 4},
		{"historical-raw", 167, 323, 0, "", 4},
		{"historical-v1", 167, 323, 0, gamestate.AuthoredSelectionProfileP2, 4},
	} {
		t.Run(q.label, func(t *testing.T) {
			c.Editor.Role = "defender"
			c.Editor.Category = q.selected
			p := gamestate.AuthoredPlacement{ID: "00000000000000000000000000000001", Role: "defender", Formation: q.fi, Component: q.ci, SelectionProfile: q.profile, X: 708, Y: 621}
			if c.editorIconCategory(p) != 9 {
				t.Fatal("source category9 required")
			}
			if row, valid := c.editorOrdinaryIconRow(p, 9); !valid || row != q.row {
				t.Fatal("source row", row, valid, q.row)
			}
			c.Editor.Document.Placements = nil
			want := c.editorFrame()
			c.Editor.Document.Placements = []gamestate.AuthoredPlacement{p}
			before := gamestate.AuthoredRevision(c.Editor.Document)
			got := c.editorFrame()
			x, y := c.editorScreen(p.X, p.Y)
			//Independent atlas composition from source rectangle/anchor literals;
			//does not call either icon painter or its row-selection helper.
			marks := [][3]int{{x, y, q.row}}
			// P5 source2223..2307: retained formation323 has FRMD+2=1;
			// its first armed component resets Y+4 on the two side marks.
			// Singleton Bunker322 becomes type2 in the private editor view.
			if q.ci == 323 {
				marks = append(marks, [3]int{x - 4, y + 5, q.row & ^4}, [3]int{x + 4, y + 5, q.row & ^4})
			}
			for _, mark := range marks {
				for yy := 0; yy < 4; yy++ {
					for xx := 0; xx < 5; xx++ {
						v := c.editorAssets().Icons[(mark[2]+yy)*320+102+xx]
						if v != 0 {
							want[(mark[1]-1+yy)*320+mark[0]-2+xx] = v
						}
					}
				}
			}
			for yy := 3; yy < 131; yy++ {
				for xx := 3; xx < 163; xx++ {
					if got[yy*320+xx] != want[yy*320+xx] {
						t.Fatal("map icon source pixel", xx, yy, got[yy*320+xx], want[yy*320+xx], "row", q.row)
					}
				}
			}
			if gamestate.AuthoredRevision(c.Editor.Document) != before || !bytes.Equal(w.FRMD, beforeFRMD) {
				t.Fatal("render mutated document/immutable components")
			}
		})
	}
	//Role markers keep their own source callers and raw atlas colors. In
	//particular this repair must not pretend the still-unclosed P/F82→15
	//difference is solved by a global recolor or ordinary weapon rule.
	for i, role := range []string{"base", "FARP", "primary", "secondary"} {
		p := gamestate.AuthoredPlacement{Role: role, Formation: 167, Component: 324, SelectionProfile: gamestate.AuthoredSelectionProfile}
		if row, ok := c.editorOrdinaryIconRow(p, 10+i); !ok || row != 0 {
			t.Fatal("role marker row changed", role, row, ok)
		}
	}
	//A selected category alone also cannot force the structure-onlyY+4 rule
	//onto another source category.
	c.Editor.Role = "defender"
	c.Editor.Category = 1
	if row, ok := c.editorOrdinaryIconRow(gamestate.AuthoredPlacement{Role: "defender"}, 0); !ok || row != 16 {
		t.Fatal("other category selection owner", row, ok)
	}
}
