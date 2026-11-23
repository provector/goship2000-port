package menus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func gulfEditor(t *testing.T) *FrontEnd {
	t.Helper()
	c := authoredUIController(t)
	d, e := gamestate.NewAuthoredDocument(c.editorData(), "GULF5.WRL", "P3 UI", 1)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.editMissionDocument(d, ""); e != nil {
		t.Fatal(e)
	}
	return c
}

func captureEditor(t *testing.T, c *FrontEnd, name string) {
	t.Helper()
	dir := os.Getenv("GS2000_P3_EDITOR_CAPTURE_DIR")
	if dir == "" {
		return
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	im, e := c.RenderRGBA(false)
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(filepath.Join(dir, name+".png"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = png.Encode(f, im); e != nil {
		t.Fatal(e)
	}
}

func TestEditorPaletteAndImmutableWorlds(t *testing.T) {
	c := gulfEditor(t)
	n := 0
	for key, m := range c.editorAssets().Maps {
		if len(m.Palette) != 256 {
			continue
		}
		c.Editor.Document.World = key + ".WRL"
		before := append(color.Palette(nil), m.Palette...)
		got := c.editorPalette()
		if got[13] != (color.NRGBA{0, 0, 0, 255}) {
			t.Fatal(key, "local13")
		}
		for i := 0; i < 256; i++ {
			if i != 13 && i != 82 && !reflect.DeepEqual(got[i], before[i]) {
				t.Fatal(key, i, "unowned DAC changed")
			}
		}
		want82 := uint8(255)
		if len(key) >= 4 && key[:4] == "ANTC" {
			want82 = 0
		}
		if got[82] != (color.NRGBA{want82, want82, want82, 255}) {
			t.Fatal(key, "source B8CE==2 RGB82", got[82])
		}
		if !reflect.DeepEqual(before, c.editorAssets().Maps[key].Palette) {
			t.Fatal("prepared palette mutated")
		}
		n++
	}
	if n != 14 {
		t.Fatal("all14 maps required", n)
	}
	//The chooser precedes70d2; it must retain the raw MIS palette until a
	//source world has actually been accepted into an editor document.
	c.Editor.Document = nil
	c.Editor.Worlds = []string{"GULF5.WRL"}
	c.Editor.WorldIndex = 0
	if !reflect.DeepEqual(c.editorPalette(), c.editorAssets().Maps["GULF5"].Palette) {
		t.Fatal("live editor RGB82 rule leaked into chooser")
	}
}

func TestEditorFormationCaptionsUseSelectedTheatrePrefix(t *testing.T) {
	c := gulfEditor(t)
	//Actual source FORM record12 differs in every theatre. Common Base or
	//Hangar labels cannot detect swapping the native0EURO/1GULF/2ANTC/3PHIL
	//domains, so those labels are insufficient as a relocation reader oracle.
	for _, q := range []struct {
		world, prefix, want string
		theatre             int
	}{
		{"EURO6.WRL", "EURO", "Buildings", 0},
		{"GULF5.WRL", "GULF", "T-72 Platoon", 1},
		{"ANTC1.WRL", "ANTC", "TAM Platoon", 2},
		{"PHIL1.WRL", "PHIL", "Scorpion Platoon", 3},
	} {
		w, err := gamestate.AuthoredWorld(c.editorData(), q.world)
		if err != nil || w.Theatre != q.theatre || int(w.THHD[0]) != q.theatre || w.WorldName[:4] != q.prefix {
			t.Fatal("source theatre carrier", q, err)
		}
		fi := -1
		for i := 0; i < len(w.FMHD)/39; i++ {
			if binary.LittleEndian.Uint16(w.FMHD[i*39+25:]) == 12 {
				fi = i
				break
			}
		}
		if fi < 0 {
			t.Fatal("source formation12 absent", q.world)
		}
		c.Editor.Document.World = q.world
		if got := c.editorPlacementCaption(gamestate.AuthoredPlacement{Formation: fi, Component: 0}, true); got != q.want {
			t.Fatal(q.world, got, q.want)
		}
	}
}
func TestEditorSourcePopupSpatialHoverAndRestore(t *testing.T) {
	c := gulfEditor(t)
	c.Editor.AutoScroll = true // explicit E288-positive auto-position/restore control.
	c.Editor.CursorX, c.Editor.CursorY = 80, 60
	rev := gamestate.AuthoredRevision(c.Editor.Document)
	captureEditor(t, c, "01-new-gulf5-controller")
	if e := c.editorArm("base", 0, 0); e != nil {
		t.Fatal(e)
	}
	e := c.Editor
	captureEditor(t, c, "02-base-hover-controller")
	if r := e.popupArea(0, 6); r != ([4]int{91, 36, 89, 6}) {
		t.Fatal("source Base caller", r)
	}
	if r := e.popupArea(2, 6); r != ([4]int{91, 28, 89, 6}) {
		t.Fatal("source Deep Strike caller", r)
	}
	c.handleEditor(Actions{Left: true})
	x, y := e.CursorX, e.CursorY
	c.handleEditor(Actions{})
	if e.CursorX != x || e.CursorY != y {
		t.Fatal("stationary frame changed logical pointer")
	}
	c.handleEditor(Actions{X: 1, Y: 1, Inside: true, PointerMoved: true, PointerPressed: true, Selector1: true})
	captureEditor(t, c, "03-base-outside-controller")
	if e.Stage != "option" || e.PopupHover != -1 {
		t.Fatal("outside accepted stale option", e.Stage, e.PopupHover)
	}
	c.handleEditor(Actions{Cancel: true})
	if e.CursorX != 80 || e.CursorY != 60 {
		t.Fatal("E288 restore", e.CursorX, e.CursorY)
	}
	if gamestate.AuthoredRevision(e.Document) != rev {
		t.Fatal("view/menu input mutated document")
	}
	if err := c.editorArm("defender", 0, 10); err != nil {
		t.Fatal(err)
	}
	captureEditor(t, c, "04-seven-singles-controller")
	if len(e.Options) != 7 || e.popupArea(6, 7) != ([4]int{4, 114, 89, 6}) {
		t.Fatal("defender source caller2/146", len(e.Options), e.popupArea(6, 7))
	}
	r := e.popupArea(1, 7)
	c.handleEditor(Actions{X: r[0], Y: r[1], Inside: true, PointerMoved: true})
	if e.PopupHover != 1 {
		t.Fatal("inclusive row start")
	}
	c.handleEditor(Actions{Down: true})
	if e.CursorY != r[1]+1 || e.PopupHover != 1 {
		t.Fatal("Down must move cursor, not linear option")
	}
	captureEditor(t, c, "05-oil-rig-hover-controller")
	frame := c.editorFrame()
	if frame[r[1]*320+r[0]+88] != 2 {
		t.Fatal("selected row background")
	}
	c.handleEditor(Actions{X: 0, Y: 0, Inside: true, PointerMoved: true})
	if e.PopupHover != -1 {
		t.Fatal("hover not released")
	}
	frame = c.editorFrame()
	if frame[143*320+174] == 1 {
		t.Fatal("blue pane retained outside valid hover")
	}
}
func TestMissionFilesPermanentChannel(t *testing.T) {
	c := authoredUIController(t)
	if e := c.openMissionFiles(); e != nil {
		t.Fatal(e)
	}
	for _, n := range []int{0, 1, 12, 13} {
		c.Files.Entries = nil
		for i := 0; i < n; i++ {
			c.Files.Entries = append(c.Files.Entries, gamestate.AuthoredEntry{ID: fmt.Sprint(i), Title: "Mission"})
		}
		c.Files.Page = 0
		frame := c.missionFilesFrame()
		control := append([]byte(nil), c.Office.Assets.Editor.Browser.Background...)
		_ = control
		// Independent style6 draw contract; no content-length predicate for channel.
		want := append([]byte(nil), frame...)
		drawPreflightFrame(c.Office.Assets.Missions, want, [4]int{299, 14, 15, 128}, 6, 0)
		if n <= 12 {
			for y := 14; y < 142; y++ {
				if !bytes.Equal(frame[y*320+299:y*320+314], want[y*320+299:y*320+314]) {
					t.Fatal("missing short/empty channel", n, y)
				}
			}
		}
	}
}
func TestEditorSignedCountDigitsPreservePanelGlyphs(t *testing.T) {
	c := gulfEditor(t)
	for _, v := range []int{0, 9, 10, -1, -10, -100, 100} {
		frame := bytes.Clone(c.editorAssets().Panel)
		before := bytes.Clone(frame)
		c.editorDigits(frame, v, 225, 132, 2)
		ink := byte(15)
		if v < 0 {
			ink = 0x44
		}
		seen := false
		for y := 0; y < 200; y++ {
			for x := 0; x < 320; x++ {
				at := y*320 + x
				if x < 224 || x > 234 || y < 132 || y > 136 {
					if frame[at] != before[at] {
						t.Fatal("numeric restore erased label/icon", x, y, v)
					}
				} else if frame[at] == ink {
					seen = true
				}
			}
		}
		if !seen {
			t.Fatal("no source digit ink", v)
		}
		want := bytes.Clone(before)
		for y := 132; y < 137; y++ {
			for x := 224; x < 235; x++ {
				want[y*320+x] = 0
			}
		}
		s := fmt.Sprintf("%02d", absInt(v))
		s = s[len(s)-2:]
		for i, digit := range []byte(s) {
			maskText(t, c, want, string(digit), 225+i*5, 132, ink, [4]int{225 + i*5, 132, 5, 5})
		}
		if !bytes.Equal(frame, want) {
			t.Fatal("source numeric fill/mask mismatch inside rectangle", v)
		}
	}
	for _, v := range []struct {
		x, y int
		want string
	}{{96, 72, "01/15"}, {-96, -72, "-1/17"}, {95, 71, "00/16"}} {
		if got := editorSourceCoordinates(v.x, v.y); got != v.want {
			t.Fatal(got, v)
		}
	}
}

// Independent expected mask composition: never calls the production text or
// numeric painters. In particular a transparent glyph's zero pixels retain the
// explicit source fill, not baked80/00 from the panel.
func maskText(t *testing.T, c *FrontEnd, dst []byte, text string, x, y int, ink byte, clip [4]int) {
	t.Helper()
	f := c.Menus.Fonts[3]
	for _, ch := range []byte(text) {
		if ch&128 != 0 {
			ink = ch & 127
			continue
		}
		for _, g := range f.Metrics.Glyphs {
			if g.Byte != int(ch) {
				continue
			}
			for yy := 0; yy < f.Metrics.Height; yy++ {
				for xx := 0; xx < g.Advance; xx++ {
					dx, dy := x+xx, y+yy
					if dx >= clip[0] && dx < clip[0]+clip[2] && dy >= clip[1] && dy < clip[1]+clip[3] && f.Mask.Pix[(g.Rect[1]+yy)*f.Mask.Stride+g.Rect[0]+xx] != 0 {
						dst[dy*320+dx] = ink
					}
				}
			}
			x += g.Advance
			break
		}
	}
}

func TestEditorNewNumericMasksReplaceBaked80And00(t *testing.T) {
	c := gulfEditor(t)
	remaining, err := gamestate.AuthoredBudget(c.editorData(), c.Editor.Document)
	counts, ce := gamestate.AuthoredDefenderCounts(c.editorData(), c.Editor.Document)
	if err != nil || ce != nil || remaining != 60 || counts[9] != 11 {
		t.Fatal("independent GULF5 NEW source builtins", remaining, counts, err, ce)
	}
	frame := c.editorFrame()
	for _, q := range []struct {
		x, y int
		text string
	}{{225, 132, "60"}, {257, 178, "11"}} {
		want := make([]byte, 64000)
		for i, digit := range []byte(q.text) {
			maskText(t, c, want, string(digit), q.x+i*5, q.y, 15, [4]int{q.x + i*5, q.y, 5, 5})
		}
		for y := q.y; y < q.y+5; y++ {
			for x := q.x - 1; x < q.x+10; x++ {
				if frame[y*320+x] != want[y*320+x] {
					t.Fatal("baked digit remained", q.text, x, y, frame[y*320+x], want[y*320+x])
				}
			}
		}
	}
	frame = bytes.Repeat([]byte{202}, 64000)
	c.editorDigitsBackground(frame, 60, 225, 132, 2, 12)
	c.editorDigitsBackground(frame, 11, 225, 132, 2, 12)
	want := bytes.Repeat([]byte{202}, 64000)
	for y := 132; y < 137; y++ {
		for x := 224; x < 235; x++ {
			want[y*320+x] = 12
		}
	}
	maskText(t, c, want, "1", 225, 132, 15, [4]int{225, 132, 5, 5})
	maskText(t, c, want, "1", 230, 132, 15, [4]int{230, 132, 5, 5})
	if !bytes.Equal(frame, want) {
		t.Fatal("active background or repaint union")
	}
}

func TestEditorParameterReadersAndExactPaintOwners(t *testing.T) {
	c := gulfEditor(t)
	w, err := gamestate.AuthoredEditorWorld(c.editorData(), "GULF5.WRL")
	if err != nil {
		t.Fatal(err)
	}
	//Independent source identities: GULF5 FMHD38 is Base(ID1); singleton
	//OBJTYPES81 is Tower, desert style. A display probe changes no document
	//compiler, placements or RNG owner through an input shortcut.
	baseComponent, towerComponent, towerFormation := -1, -1, -1
	baseID := int(binary.LittleEndian.Uint16(w.FMHD[38*39+25:]))
	for i := 0; i < len(w.FRMD)/8; i++ {
		if int(w.FRMD[i*8]) == baseID && baseComponent < 0 {
			baseComponent = i
		}
		if w.FRMD[i*8+1] == 81 && w.FRMD[i*8+2] != 1 && towerComponent < 0 {
			towerComponent = i
		}
	}
	if towerComponent >= 0 {
		at := towerComponent
		for at > 0 && w.FRMD[at*8] == 255 {
			at--
		}
		id := int(w.FRMD[at*8])
		for i := 0; i < len(w.FMHD)/39; i++ {
			if int(binary.LittleEndian.Uint16(w.FMHD[i*39+25:])) == id {
				towerFormation = i
				break
			}
		}
	}
	if baseID != 1 || baseComponent < 0 || towerComponent < 0 || towerFormation < 0 {
		t.Fatal("source caption identities", baseID, baseComponent, towerComponent, towerFormation)
	}
	c.Editor.Document.Placements = []gamestate.AuthoredPlacement{
		{Role: "base", Formation: 38, Component: baseComponent, X: 1152, Y: 792},
		{Role: "FARP", Formation: 38, Component: baseComponent, X: 96, Y: 72},
		{Role: "primary", Kind: 6, Formation: towerFormation, Component: towerComponent, X: 1152, Y: 792},
		{Role: "secondary", Kind: 4, Formation: towerFormation, Component: towerComponent, X: -96, Y: -72},
	}
	got := bytes.Repeat([]byte{77}, 64000)
	c.drawEditorParameters(got)
	want := bytes.Repeat([]byte{77}, 64000)
	for _, q := range []struct {
		r    [4]int
		text string
		ink  byte
	}{
		// P4 adds the missing Support parameter producer. Independent1DA4:1B78
		// pushes colourCA,height6,width50h,y42h,xD6h for this exact area.
		{[4]int{214, 66, 80, 6}, "None", 15},
		{[4]int{198, 12, 120, 6}, "Base located at 12/05", 15},
		{[4]int{198, 22, 120, 5}, "Located at 01/15", 15},
		{[4]int{214, 32, 80, 6}, "Recon", 15},
		{[4]int{178, 40, 130, 6}, "Tower, desert style " + string([]byte{0x8f}) + "at 12/04", 0x44},
		{[4]int{224, 49, 80, 6}, "Point Attack", 15},
		{[4]int{178, 57, 130, 6}, "Tower, desert style " + string([]byte{0x8f}) + "at -1/16", 0x44},
	} {
		for y := q.r[1]; y < q.r[1]+q.r[3]; y++ {
			for x := q.r[0]; x < q.r[0]+q.r[2]; x++ {
				want[y*320+x] = 202
			}
		}
		maskText(t, c, want, q.text, q.r[0], q.r[1], q.ink, q.r)
	}
	if !bytes.Equal(got, want) {
		for i := range got {
			if got[i] != want[i] {
				t.Fatal("parameter glyph/background owner", i%320, i/320, got[i], want[i])
			}
		}
	}
	for y := 0; y < 200; y++ {
		for x := 0; x < 166; x++ {
			if got[y*320+x] != 77 {
				t.Fatal("parameter repaint touched map", x, y)
			}
		}
	}
}

func TestEditorModalRaisedAndCategoryDepressed(t *testing.T) {
	c := gulfEditor(t)
	c.Editor.Stage = "exit"
	frame := c.editorFrame()
	for key, r := range c.editorExitAreas(false) {
		if frame[r[1]*320+r[0]+r[2]-2] != 15 || frame[(r[1]+r[3])*320+r[0]+1] != 8 {
			t.Fatal("modal idle must be state0 raised", key, r)
		}
	}
	if frame[60*320+120] != 8 || frame[150*320+120] != 15 {
		t.Fatal("independent729e outer dialog border changed")
	}
	c.Editor.Stage = "edit"
	c.Editor.Role = "defender"
	c.Editor.Category = 10
	frame = c.editorFrame()
	for _, r := range c.editorAssets().Rectangles {
		if r.Group == "category_and_view" && r.Index == 10 {
			if frame[r.Y*320+r.X+r.Width-2] != 8 || frame[(r.Y+r.Height)*320+r.X+1] != 15 {
				t.Fatal("selected category lost state1", r)
			}
		}
	}
}

func TestEditorStatusSourcePaletteClock(t *testing.T) {
	c := &FrontEnd{Editor: &MissionEditor{Status: "GUNSHIP 2000: Mission Builder"}}
	c.publishEditorStatus()
	e := c.Editor
	for _, v := range []struct {
		age uint32
		red uint8
	}{{0, 0}, {1, 130}, {5, 146}, {17, 195}, {31, 243}, {32, 243}, {64, 243}} {
		e.StatusUIAge = v.age
		if got := e.editorStatusRed(); got != v.red {
			t.Fatal(v.age, got, v.red)
		}
	}
	e.StatusUIAge = 31
	e.Status = "New source message"
	c.publishEditorStatus()
	if e.StatusUIAge != 0 || !e.StatusShown {
		t.Fatal("new message did not reset source phase")
	}
	c.advanceEditorStatus()
	if e.StatusUIAge != 1 || e.editorStatusRed() != 130 {
		t.Fatal("UI tick did not start source red ramp")
	}
	before := e.StatusUIAge
	for i := 0; i < 20; i++ {
		_ = e.editorStatusRed()
	}
	if e.StatusUIAge != before {
		t.Fatal("presentation getter advanced status")
	}
}

func TestEditorConditionalPopupAndProcessAutoScroll(t *testing.T) {
	c := gulfEditor(t)
	e := c.Editor
	e.CursorX, e.CursorY = 80, 60
	if e.AutoScroll {
		t.Fatal("fresh process invented E288")
	}
	if err := c.editorArm("base", 0, 0); err != nil {
		t.Fatal(err)
	}
	if e.CursorX != 80 || e.CursorY != 60 || e.PopupHover != -1 {
		t.Fatal("E288zero warped/selected", e.CursorX, e.CursorY, e.PopupHover)
	}
	c.handleEditor(Actions{Enter: true})
	if e.Stage != "option" {
		t.Fatal("outside keyboard accept")
	}
	c.handleEditor(Actions{Cancel: true})
	c.handleEditor(Actions{Text: []rune{'a'}})
	if !e.AutoScroll || !c.editorAutoScroll {
		t.Fatal("process toggle owner")
	}
	if err := c.editorArm("base", 0, 0); err != nil {
		t.Fatal(err)
	}
	if e.PopupHover != 0 {
		t.Fatal("E288positive did not position")
	}
	c.handleEditor(Actions{Cancel: true})
	d, _ := gamestate.CloneAuthored(e.Document)
	if err := c.editMissionDocument(d, ""); err != nil {
		t.Fatal(err)
	}
	if !c.Editor.AutoScroll {
		t.Fatal("EDIT lost process E288")
	}
}
func TestEditorTypematicMakesAndReleaseBoundaries(t *testing.T) {
	c := &FrontEnd{Screen: ScreenMissionEditor, Editor: &MissionEditor{Stage: "option", CursorX: 48, CursorY: 138, PopupX: 2, PopupBottom: 146, Options: make([]gamestate.AuthoredOption, 7)}}
	up := [4]bool{false, false, true, false}
	none := [4]bool{}
	apply := func(a Actions, held, press, repeat [4]bool) {
		a = c.editorArrowMakes(a, held, press, repeat)
		c.editorMoveCursor(a, false)
		c.editorPopupHover(7)
	}
	apply(Actions{}, none, none, none)
	apply(Actions{}, up, up, none)
	if c.Editor.CursorY != 137 {
		t.Fatal("initial make")
	}
	apply(Actions{}, up, none, none)
	if c.Editor.CursorY != 137 {
		t.Fatal("hold invented make")
	}
	apply(Actions{}, up, none, up)
	if c.Editor.CursorY != 136 {
		t.Fatal("typematic make missing")
	}
	// From row0 y138, two Up makes reach y136: row2's inclusive
	// bottom edge. Linear previous-index navigation would select6/5.
	if c.Editor.Option != 2 {
		t.Fatal("repeat ignored source spatial row", c.Editor.Option)
	}
	c.Editor.Stage = "kind"
	apply(Actions{}, up, none, up)
	if c.Editor.CursorY != 136 {
		t.Fatal("stage transition leak")
	}
	apply(Actions{}, none, none, none)
	apply(Actions{}, up, up, none)
	if c.Editor.CursorY != 135 {
		t.Fatal("release did not rearm")
	}
	apply(Actions{FlightFocusLost: true}, up, none, up)
	apply(Actions{}, up, none, up)
	if c.Editor.CursorY != 135 {
		t.Fatal("focus leak")
	}
	apply(Actions{}, none, none, none)
	apply(Actions{}, up, up, none)
	if c.Editor.CursorY != 134 {
		t.Fatal("focus rearm")
	}
	c.Screen = ScreenQuit
	apply(Actions{}, up, none, up)
	c.Screen = ScreenMissionEditor
	apply(Actions{}, up, none, up)
	if c.Editor.CursorY != 134 {
		t.Fatal("modal return leak")
	}
	// Selector edges are never manufactured by this direction-only transport.
	a := c.editorArrowMakes(Actions{}, up, none, up)
	if a.Enter || a.Selector1 || a.Selector2 {
		t.Fatal("repeat manufactured SAVE/BEGIN selector")
	}
}
