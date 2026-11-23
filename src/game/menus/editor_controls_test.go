package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestRetainedDetailPathClippingKeepsRasterPhase(t *testing.T) {
	c := &FrontEnd{Editor: &MissionEditor{Document: &gamestate.AuthoredDocument{Seed: 71, Editor: &gamestate.AuthoredEditorState{Profile: gamestate.AuthoredControlProfile, Stream: 123, FillBoundary: 2}}}}
	c.Editor.View.Detail = true
	c.Editor.View.Bounds = [4]int{170, 65, 147, 123}
	before, _ := json.Marshal(c.Editor.Document)
	for _, tc := range []struct {
		name  string
		line  [4]int
		shift [2]int
	}{
		{"left crossing", [4]int{-15, 70, 300, 110}, [2]int{15, 0}},
		{"top crossing", [4]int{200, -15, 260, 160}, [2]int{0, 15}},
		{"steep reverse", [4]int{260, 160, 200, -15}, [2]int{0, 15}},
		{"inclusive corners", [4]int{170, 65, 317, 188}, [2]int{}},
		{"upper equality", [4]int{170, 65, 317, 65}, [2]int{}},
		{"outside equality", [4]int{170, 64, 317, 64}, [2]int{}},
		{"endpoint equality", [4]int{317, 188, 317, 188}, [2]int{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bytes.Repeat([]byte{0x27}, 64000)
			reference := make([]byte, 64000)
			want := bytes.Repeat([]byte{0x27}, 64000)
			if err := assetdata.OfficeLine(reference, tc.line[0]+tc.shift[0], tc.line[1]+tc.shift[1], tc.line[2]+tc.shift[0], tc.line[3]+tc.shift[1], 3); err != nil {
				t.Fatal(err)
			}
			// Translation preserves OfficeLine's full-segment error phase. Crop its
			// independently drawn result into the detail viewport after translation.
			for y := 65; y <= 188; y++ {
				for x := 170; x <= 317; x++ {
					rx, ry := x+tc.shift[0], y+tc.shift[1]
					if rx >= 0 && rx < 320 && ry >= 0 && ry < 200 && reference[ry*320+rx] == 3 {
						want[y*320+x] = 3
					}
				}
			}
			c.editorClippedControlLine(got, tc.line[0], tc.line[1], tc.line[2], tc.line[3], 3)
			if !bytes.Equal(got, want) {
				for i, v := range got {
					if v != want[i] {
						t.Fatalf("pixel(%d,%d)=%d want%d", i%320, i/320, v, want[i])
					}
				}
			}
		})
	}
	frame := make([]byte, 64000)
	c.editorClippedControlLine(frame, -20, 100, 340, 100, 3)
	for x := 0; x < 320; x++ {
		want := byte(0)
		if x >= 170 && x <= 317 {
			want = 3
		}
		if frame[100*320+x] != want {
			t.Fatalf("both endpoints outside at%d", x)
		}
	}
	invalid := make([]byte, 64000)
	c.editorClippedControlLine(invalid, -1000000000, 100, 340, 100, 3)
	if bytes.Count(invalid, []byte{3}) != 0 {
		t.Fatal("unbounded line admitted")
	}
	after, _ := json.Marshal(c.Editor.Document)
	if !bytes.Equal(before, after) {
		t.Fatal("clipped path changed document or stored RNG")
	}
}

func controlsEditor(t *testing.T) *FrontEnd {
	t.Helper()
	c := authoredUIController(t)
	if err := c.newEditor(); err != nil {
		t.Fatal(err)
	}
	if err := c.chooseEditorWorld(0); err != nil {
		t.Fatal(err)
	}
	if c.Editor.Document == nil {
		t.Fatal(c.Editor.Status)
	}
	for _, r := range []struct {
		role       string
		kind, x, y int
	}{{"base", 0, 800, 500}, {"primary", 7, 600, 600}, {"secondary", 3, 800, 600}} {
		opts, err := gamestate.AuthoredOptions(c.editorData(), c.Editor.Document, r.role, r.kind)
		if err != nil || len(opts) == 0 {
			t.Fatal(opts, err)
		}
		if err = gamestate.PlaceAuthored(c.editorData(), c.Editor.Document, r.role, r.kind, opts[0].Formation, r.x, r.y); err != nil {
			t.Fatal(err)
		}
	}
	c.Editor.Stage = "edit"
	return c
}

func TestControlSourceMenusAllChoicesCancelAndCommit(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	if len(c.editorAssets().FillLabels) != 6 || len(c.editorAssets().DeleteLabels) != 19 {
		t.Fatal("typed source menu resources missing")
	}
	for _, menu := range []struct {
		name  string
		count int
	}{{"fill", 6}, {"delete", 19}} {
		for slot := 0; slot < menu.count; slot++ {
			t.Run(fmt.Sprintf("%s/%d", menu.name, slot), func(t *testing.T) {
				before, _ := json.Marshal(e.Document)
				if err := c.editorOpenControlMenu(menu.name); err != nil {
					t.Fatal(err)
				}
				// Source3430 pairs options bottom-up, rather than Down=next.
				// Use the actual caller (Fill88/188, Delete128/188) and
				// inclusive90x7paint/89x6hit domains as independent operands.
				origin := 88
				if menu.name == "delete" {
					origin = 128
				}
				x, y := origin+2+95*(slot%2)+2, 188-8*(slot/2+1)+2
				handled, err := c.handleEditorControlPopup(Actions{Inside: true, X: x, Y: y, PointerMoved: true}, true, false, false)
				if !handled || err != nil || e.ControlChoice != slot {
					t.Fatal(handled, err, e.ControlChoice, slot)
				}
				// One keyboard make moves1pixel in the same source row. A
				// parked physical pointer must not overwrite that position.
				c.handleEditorControlPopup(Actions{Down: true}, false, false, false)
				c.handleEditorControlPopup(Actions{Inside: true, X: x, Y: y}, false, false, false)
				if e.ControlChoice != slot || e.CursorX != x || e.CursorY != y+1 {
					t.Fatal("parked pointer/spatial row", e.ControlChoice, e.CursorX, e.CursorY)
				}

				frame := make([]byte, 64000)
				c.drawEditorControlOverlay(frame)
				if bytes.Equal(frame, make([]byte, 64000)) {
					t.Fatal("menu did not draw")
				}
				c.handleEditorControlPopup(Actions{Cancel: true}, false, false, false)
				after, _ := json.Marshal(e.Document)
				if !bytes.Equal(before, after) {
					t.Fatal("cancel mutated document/stream/cache")
				}
			})
		}
	}
	// Real UI transaction, not just a guard/status response.
	if err := c.editorOpenControlMenu("fill"); err != nil {
		t.Fatal(err)
	}
	// Explicit source row0 hit; E288 defaults off in a fresh process.
	c.handleEditorControlPopup(Actions{Inside: true, X: 92, Y: 182, PointerMoved: true}, true, false, false)
	if _, err := c.handleEditorControlPopup(Actions{Enter: true}, false, true, false); err != nil {
		t.Fatal(err)
	}
	if e.Document.Editor == nil || !e.Dirty {
		t.Fatal("interactive Fill did not mutate draft", e.Status)
	}
	c.editorOpenControlMenu("delete")
	c.handleEditorControlPopup(Actions{Inside: true, X: 227, Y: 182, PointerMoved: true}, true, false, false)
	c.handleEditorControlPopup(Actions{Enter: true}, false, true, false)
	if e.Stage != "control_delete_confirm" {
		t.Fatal(e.Stage)
	}
	before, _ := json.Marshal(e.Document)
	c.handleEditorControlPopup(Actions{Text: []rune("n")}, false, false, false)
	after, _ := json.Marshal(e.Document)
	if !bytes.Equal(before, after) {
		t.Fatal("NO removed defenders")
	}
	c.editorOpenControlMenu("delete")
	c.handleEditorControlPopup(Actions{Inside: true, X: 227, Y: 182, PointerMoved: true}, true, false, false)
	c.handleEditorControlPopup(Actions{Enter: true}, false, true, false)
	c.handleEditorControlPopup(Actions{Text: []rune("y")}, false, false, false)
	for _, p := range e.Document.Placements {
		if p.Filled {
			t.Fatal("Filled objects source Delete left filled placement")
		}
	}
}

func TestFilterAllAndSelectorComponentViewIsolation(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	revision := gamestate.AuthoredRevision(e.Document)
	c.editorControlCommand(14)
	if e.Filter != 0x3ff || e.Stage != "filter" {
		t.Fatal(e.Filter, e.Stage)
	}
	c.handleEditorControlPopup(Actions{Down: true}, false, false, false)
	c.handleEditorControlPopup(Actions{Selector1: true}, false, true, false)
	want := uint16(0x3fd)
	if e.Filter != want || c.editorCategoryVisible(1) {
		t.Fatal(e.Filter)
	}
	c.handleEditorControlPopup(Actions{Enter: true}, false, false, false)
	e.Weapons, e.Paths = false, true
	c.editorControlCommand(15)
	if !e.All || !c.editorCategoryVisible(1) || e.Weapons || !e.Paths || e.Filter != want {
		t.Fatal("All altered underlying flags")
	}
	c.editorControlCommand(15)
	if e.All || c.editorCategoryVisible(1) || e.Filter != want || e.Weapons || !e.Paths {
		t.Fatal("All did not restore")
	}
	// Source F4 is an AREA toggle, not a physical-button hold. The normal
	// controller receives an activation edge, then idle/release retains it.
	e.Stage = "edit"
	if err := c.Handle(Actions{ViewKey: 4}); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{}); err != nil {
		t.Fatal(err)
	}
	if !e.All || e.Filter != want || e.Weapons || !e.Paths {
		t.Fatal("All changed on release or underlying flags")
	}
	if err := c.Handle(Actions{ViewKey: 4}); err != nil {
		t.Fatal(err)
	}
	if e.All || e.Filter != want {
		t.Fatal("second F4 did not restore")
	}
	for _, formation := range []bool{false, true} {
		e.Formation = formation
		found := false
		for cat := 1; cat <= 10; cat++ {
			opts, err := c.editorOptions("defender", 0, cat)
			if err != nil {
				t.Fatal(err)
			}
			if len(opts) == 0 {
				continue
			}
			e.Options, e.Option, e.Stage = opts, 0, "option"
			c.editorBeginPopup(2, 146, len(opts))
			c.handleEditor(Actions{Inside: true, X: 6, Y: 140, PointerMoved: true})
			rows, err := c.editorPreviewRows()
			if err != nil || len(rows) == 0 {
				t.Fatal(rows, err)
			}
			old := e.ComponentPreview
			c.editorCyclePreview()
			if e.ComponentPreview != (old+1)%len(rows) {
				t.Fatal(e.ComponentPreview)
			}
			frame := make([]byte, 64000)
			c.drawEditorControlOverlay(frame)
			if bytes.Count(frame, []byte{1}) == 0 {
				t.Fatal("blue component pane missing")
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("no positive source selection witness for formation=%v", formation)
		}
	}
	c.editorControlCommand(17)
	if e.ControlReturn != "information" {
		t.Fatal("help context missing")
	}
	c.editorControlCommand(17)
	if e.ControlReturn != "" {
		t.Fatal("help toggle not restored")
	}
	if gamestate.AuthoredRevision(e.Document) != revision || e.Document.Editor != nil {
		t.Fatal("display controls changed persistent/RNG owners")
	}
}

func TestSingleComponentPickupRetainsExactSelection(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	w, _ := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	opts, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range opts {
		if o.Fixed || w.FRMD[o.Component*8] != 255 {
			continue
		}
		if err = gamestate.PlaceAuthoredOption(c.editorData(), e.Document, "defender", 0, o, 400, 400); err != nil {
			continue
		}
		p := e.Document.Placements[len(e.Document.Placements)-1]
		e.CursorX, e.CursorY = c.editorScreen(p.X, p.Y)
		e.Held = false
		if err = c.editorPickUp(); err != nil {
			t.Fatal(err)
		}
		if !e.Held || e.Option < 0 || e.Option >= len(e.Options) {
			t.Fatal("single variant was not picked up", e.Status)
		}
		picked := e.Options[e.Option]
		if picked.Formation != p.Formation || picked.Component != p.Component || picked.Template != p.Template {
			t.Fatal("pickup changed source component", p, picked)
		}
		for _, q := range e.Document.Placements {
			if q.ID == p.ID {
				t.Fatal("pickup did not remove exact source descriptor")
			}
		}
		found = true
		break
	}
	if !found {
		t.Fatal("source alternate single component witness missing")
	}
}

func TestSourceWeaponCircleAndRetainedPathPresentation(t *testing.T) {
	c := controlsEditor(t)
	e := c.Editor
	e.Weapons = true
	frame := make([]byte, 64000)
	c.editorControlCircle(frame, 40, 40, 3, 0x9f)
	for _, p := range [][2]int{{43, 40}, {37, 40}, {40, 43}, {40, 37}} {
		if frame[p[1]*320+p[0]] != 0x9f {
			t.Fatal("source radius cardinal missing", p)
		}
	}
	frame = make([]byte, 64000)
	c.editorControlCircle(frame, 3, 3, 8, 0x9f)
	for y := 0; y < 3; y++ {
		if bytes.Count(frame[y*320:(y+1)*320], []byte{0x9f}) != 0 {
			t.Fatal("circle escaped map clip")
		}
	}
	// Source weapon class0 has radius0. Two actual nibbles source-read through
	// typed27-byte tables; no kilometres or range conversion is introduced.
	w, _ := gamestate.AuthoredWorld(c.editorData(), e.Document.World)
	opts, _, err := gamestate.AuthoredEditorOptions(c.editorData(), e.Document, "defender", 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	armed := false
	for _, o := range opts {
		b := w.FRMD[o.Component*8:]
		if b[3] == 0 {
			continue
		}
		if err = gamestate.PlaceAuthoredOption(c.editorData(), e.Document, "defender", 0, o, 400, 400); err != nil {
			continue
		}
		armed = true
		break
	}
	if !armed {
		t.Fatal("source armed static witness missing")
	}
	frame = make([]byte, 64000)
	c.drawEditorEffectiveness(frame)
	if bytes.Count(frame, []byte{0x9f}) == 0 {
		t.Fatal("Weapons toggle has no mechanical display")
	}
	endpoint := gamestate.MissionPoint{700 * 341, -500*455 + 0x7ffff}
	e.Document.Initial = &gamestate.AuthoredInitialCarrier{Layout: gamestate.AuthoredRealization{Objectives: map[string]gamestate.MissionObjective{"primary": {Point: gamestate.MissionPoint{400 * 341, -400*455 + 0x7ffff}, Endpoint: &endpoint}}}}
	before, _ := json.Marshal(e.Document)
	frame = make([]byte, 64000)
	c.drawEditorPaths(frame)
	if bytes.Count(frame, []byte{3}) == 0 {
		t.Fatal("retained source destination path missing")
	}
	after, _ := json.Marshal(e.Document)
	if !bytes.Equal(before, after) {
		t.Fatal("path display edited retained carrier")
	}
}
