package menus

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	gamestate "gunship2000-assets/game/state"
	"strings"
	"testing"
)

func TestIconSameControlExitAndInputOnce(t *testing.T) {
	c := controlsEditor(t)
	before, _ := json.Marshal(c.Editor.Document)
	var icon [2]int
	for _, r := range c.editorAssets().Rectangles {
		if r.Group == "category_and_view" && r.Index == 14 {
			icon = [2]int{r.X + r.Width/2, r.Y + r.Height/2}
		}
	}
	//The same F3 that opens ICON must close it without applying an unrelated
	//category or leaking a placement edge into the returned editor.
	for n := 0; n < 2; n++ {
		if err := c.handleEditor(Actions{ViewKey: 3}); err != nil {
			t.Fatal(err)
		}
		want := "filter"
		if n == 1 {
			want = "edit"
		}
		if c.Editor.Stage != want {
			t.Fatal(n, c.Editor.Stage)
		}
	}
	if err := c.handleEditor(Actions{PointerPressed: true, Inside: true, X: icon[0], Y: icon[1], Selector1: true}); err != nil {
		t.Fatal(err)
	}
	if c.Editor.Stage != "filter" {
		t.Fatal(c.Editor.Stage)
	}
	filter := c.Editor.Filter
	if err := c.handleEditor(Actions{PointerPressed: true, Inside: true, X: icon[0], Y: icon[1], Selector1: true}); err != nil {
		t.Fatal(err)
	}
	if c.Editor.Stage != "edit" || c.Editor.Filter != filter {
		t.Fatal("ICON changed current category", c.Editor.Stage, c.Editor.Filter, filter)
	}
	c.editorControlCommand(14)
	if err := c.handleEditor(Actions{Cancel: true}); err != nil || c.Editor.Stage != "edit" {
		t.Fatal(err, c.Editor.Stage)
	}
	after, _ := json.Marshal(c.Editor.Document)
	if !bytes.Equal(before, after) {
		t.Fatal("view mode mutated document/RNG")
	}
}

func TestOverviewAndDetailHaveIndependentSymbolOwners(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	options, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 3, true)
	if err != nil || len(options) == 0 {
		t.Fatal(options, err)
	}
	o := options[0]
	p := gamestate.AuthoredPlacement{Role: "defender", Formation: o.Formation, Component: o.Component, Template: o.Template, SelectionProfile: o.SelectionProfile, X: 108, Y: 81}
	e.Document.Placements = []gamestate.AuthoredPlacement{p}
	e.Role, e.Category = "", 0
	e.View.Detail = true
	e.View.CameraX = -170
	e.View.CameraY = 1017
	e.View.Bounds = [4]int{170, 65, 147, 123}
	e.FilterInitialized = true
	e.Filter = 0x3ff
	before, _ := json.Marshal(e.Document)
	frame := make([]byte, 64000)
	e.paintOverview = true
	c.drawEditorPlacedSymbols(frame, true)
	//Raw source540A writes exactly one center and a type1's two tiny flanks.
	x, y := editorOverviewScreen(108, 81)
	for _, at := range [][2]int{{x, y}, {x - 1, y + 1}, {x + 1, y + 1}} {
		if frame[at[1]*320+at[0]] != 68 {
			t.Fatal("overview source anchor", at, frame[at[1]*320+at[0]])
		}
	}
	if bytes.Count(frame, []byte{68}) != 3 {
		t.Fatal("overview substituted a detail silhouette")
	}
	e.paintOverview = false
	c.drawEditorPlacedSymbols(frame, false)
	dx, dy := e.View.screen(p.X, p.Y)
	if dx < 170 || dx > 317 || dy < 65 || dy > 188 {
		t.Fatal(dx, dy)
	}
	detail := 0
	for yy := 65; yy <= 188; yy++ {
		for xx := 170; xx <= 317; xx++ {
			if frame[yy*320+xx] != 0 {
				detail++
			}
		}
	}
	if detail <= 3 {
		t.Fatal("detail atlas not drawn", detail)
	}
	if frame[y*320+x] != 68 {
		t.Fatal("detail erased fixed overview")
	}
	//ICON's mutable callbacks are source5980 overview owners; detail230E
	//always visits the atlas registry and must remain visible when filtered.
	e.Filter = 0
	filtered := make([]byte, 64000)
	e.paintOverview = true
	c.drawEditorPlacedSymbols(filtered, true)
	e.paintOverview = false
	c.drawEditorPlacedSymbols(filtered, false)
	if filtered[y*320+x] != 0 {
		t.Fatal("filtered overview still visible")
	}
	for yy := 65; yy <= 188; yy++ {
		if !bytes.Equal(frame[yy*320+170:yy*320+318], filtered[yy*320+170:yy*320+318]) {
			t.Fatal("overview filter changed independent detail", yy)
		}
	}
	e.All = true
	e.paintOverview = true
	c.drawEditorPlacedSymbols(filtered, true)
	e.paintOverview = false
	if filtered[y*320+x] != 68 || e.Filter != 0 {
		t.Fatal("ALL did not temporarily override retained filter")
	}
	//Filled provenance is not a different symbol/classification domain.
	e.Document.Placements[0].Filled = true
	f := make([]byte, 64000)
	e.paintOverview = true
	c.drawEditorPlacedSymbols(f, true)
	e.paintOverview = false
	if bytes.Count(f, []byte{68}) != 3 {
		t.Fatal("Fill changed source glyph mapping")
	}
	e.Document.Placements[0].Filled = false
	after, _ := json.Marshal(e.Document)
	if !bytes.Equal(before, after) {
		t.Fatal("drawing changed authored state/RNG")
	}
}

func TestEveryWorldSymbolRegistryAndClip(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	if len(c.editorAssets().Maps) != 14 {
		t.Fatal("fourteen source editor map prerequisite", len(c.editorAssets().Maps))
	}
	for key, m := range c.editorAssets().Maps {
		if m.Unavailable != "" {
			t.Logf("%s known decode exclusion: %s", key, m.Unavailable)
			continue
		}
		if len(m.Palette) != 256 {
			t.Fatal(key, "selected-world palette missing")
		}
	}
	checked := 0
	types := c.editorData().Tables.EditorObjectTypes
	for _, w := range c.editorData().Worlds {
		if _, present := c.editorAssets().Maps[strings.TrimSuffix(w.WorldName, ".WRL")]; !present {
			continue
		}
		checked++
		e.Document.World = w.WorldName
		e.FilterInitialized = true
		e.Filter = 0x3ff
		e.All = false
		e.Role = ""
		e.Category = 0
		seen := map[int]bool{}
		for fi := 0; fi < len(w.FMHD)/39; fi++ {
			id := int(binary.LittleEndian.Uint16(w.FMHD[fi*39+25:]))
			ci := -1
			for j := 0; j < len(w.FRMD)/8; j++ {
				if int(w.FRMD[j*8]) == id {
					ci = j
					break
				}
			}
			if ci < 0 {
				continue
			}
			model := int(w.FRMD[ci*8+1])
			if model*27+1 >= len(types) {
				continue
			}
			cat := min(9, int(types[model*27+1]))
			if seen[cat] {
				continue
			}
			seen[cat] = true
			p := gamestate.AuthoredPlacement{Role: "defender", Formation: fi, Component: ci, SelectionProfile: gamestate.AuthoredSelectionProfileP5, X: 108, Y: 81}
			e.Document.Placements = []gamestate.AuthoredPlacement{p}
			for _, detail := range []bool{false, true} {
				e.View = newEditorMapView(1)
				e.View.Detail = detail
				e.View.CameraX = -170
				e.View.CameraY = 1017
				e.paintOverview = !detail
				f := make([]byte, 64000)
				c.drawEditorPlacedSymbols(f, !detail)
				b := c.editorMapBounds()
				ink := 0
				for y := 0; y < 200; y++ {
					for x := 0; x < 320; x++ {
						if f[y*320+x] != 0 {
							ink++
							if !editorInclusive(x, y, b) {
								t.Fatal(w.WorldName, cat, "outside actual view", x, y, b)
							}
						}
					}
				}
				if ink == 0 {
					t.Fatal(w.WorldName, cat, "source registry missing", detail)
				}
				//Move the anchor onto each view edge; formation flanks and atlas crops
				//must remain clipped to that same owner rather than bleeding into UI.
				for _, at := range [][2]int{{b[0], b[1]}, {b[0] + b[2], b[1] + b[3]}} {
					p.X, p.Y = editorOverviewPoint(at[0], at[1])
					if detail {
						p.X, p.Y = e.View.point(at[0], at[1])
					}
					e.Document.Placements[0] = p
					edge := make([]byte, 64000)
					c.drawEditorPlacedSymbols(edge, !detail)
					for y := 0; y < 200; y++ {
						for x := 0; x < 320; x++ {
							if edge[y*320+x] != 0 && !editorInclusive(x, y, b) {
								t.Fatal(w.WorldName, cat, "edge crop", x, y, b)
							}
						}
					}
				}
				e.Document.Placements[0].X, e.Document.Placements[0].Y = 108, 81
			}
		}
		t.Logf("%s source categories %v", w.WorldName, seen)
	}
	if checked == 0 {
		t.Fatal("no matched generation/editor map")
	}
	t.Logf("validated %d source world registries across14 editor map records", checked)
	e.paintOverview = false
}

func TestDeepStrikeBarIsAdditionalAndClipped(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	for _, q := range []struct {
		p          [2]int
		horizontal bool
	}{{[2]int{780, 45}, true}, {[2]int{60, 630}, false}} {
		p := gamestate.AuthoredPlacement{Role: "base", X: q.p[0], Y: q.p[1], Entry: &gamestate.AuthoredDeepStrikeEntry{Profile: gamestate.AuthoredEntryProfile}}
		e.View.Detail = false
		f := make([]byte, 64000)
		c.editorEntrySymbol(f, p, true)
		x, y := editorOverviewScreen(p.X, p.Y)
		for n := -12; n <= 20; n++ {
			xx, yy := x+4, y+n
			if q.horizontal {
				xx, yy = x+n, y+4
			}
			if editorInclusive(xx, yy, c.editorMapBounds()) && f[yy*320+xx] != 1 {
				t.Fatal("source overview edge bar", xx, yy)
			}
		}
		e.View = newEditorMapView(1)
		e.View.Detail = true
		e.View.CameraX = float32(p.X - 240)
		e.View.CameraY = float32(952 - p.Y + 120)
		f = make([]byte, 64000)
		c.editorEntrySymbol(f, p, false)
		x, y = e.View.screen(p.X, p.Y)
		b := c.editorMapBounds()
		for n := -40; n <= 40; n++ {
			xx, yy := x, y+n
			if q.horizontal {
				xx, yy = x+n, y
			}
			if xx >= b[0] && xx < b[0]+b[2] && yy >= b[1] && yy < b[1]+b[3] && f[yy*320+xx] != 1 {
				t.Fatal("source detail40 edge bar", xx, yy)
			}
		}
	}
}

func TestEveryFillChoiceThroughActualPopupConsumer(t *testing.T) {
	c := controlsEditor(t)
	original, err := gamestate.CloneAuthored(c.Editor.Document)
	if err != nil {
		t.Fatal(err)
	}
	for slot := 0; slot < 6; slot++ {
		c.Editor.Document, err = gamestate.CloneAuthored(original)
		if err != nil {
			t.Fatal(err)
		}
		c.Editor.Dirty = false
		c.Editor.Stage = "edit"
		if err = c.editorOpenControlMenu("fill"); err != nil || c.Editor.Stage != "fill" {
			t.Fatal(slot, err, c.Editor.Status)
		}
		//3430 pairs bottom-up; acceptance is the actual selector1 press owner.
		x, y := 90+95*(slot%2)+2, 188-8*(slot/2+1)+2
		if _, err = c.handleEditorControlPopup(Actions{Inside: true, X: x, Y: y, PointerMoved: true}, true, false, false); err != nil || c.Editor.ControlChoice != slot {
			t.Fatal(slot, err, c.Editor.ControlChoice)
		}
		before := len(c.Editor.Document.Placements)
		if _, err = c.handleEditorControlPopup(Actions{Inside: true, X: x, Y: y, PointerPressed: true, Selector1: true}, false, true, false); err != nil || c.Editor.Stage != "edit" || c.Editor.Status != "" || !c.Editor.Dirty || len(c.Editor.Document.Placements) <= before {
			t.Fatal(slot, err, c.Editor.Stage, c.Editor.Status)
		}
		frozen, _ := json.Marshal(c.Editor.Document)
		if err = c.handleEditor(Actions{Inside: true, X: x, Y: y, PointerLeftHeld: true, PointerStateValid: true}); err != nil {
			t.Fatal(err)
		}
		held, _ := json.Marshal(c.Editor.Document)
		if !bytes.Equal(frozen, held) {
			t.Fatal(slot, "held selector repeated Fill")
		}
	}
}
