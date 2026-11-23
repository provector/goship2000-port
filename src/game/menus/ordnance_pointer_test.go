package menus

import (
	"bytes"
	"encoding/hex"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestOrdnanceRowSelectorConsumesRawMotionOnce(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	before := v.Working
	_, _ = v.Handle(Actions{X: 300, Y: 100, Inside: true})
	initial := v.rowPointer.y
	_, _ = v.Handle(Actions{X: 0, Y: 100, Inside: true})
	if v.rowPointer.y != initial {
		t.Fatal("horizontal motion changed a vertical selector")
	}
	for y := 101; y <= 250; y++ {
		_, _ = v.Handle(Actions{X: 0, Y: y, Inside: true})
	}
	if v.rowPointer.y != 181 {
		t.Fatal("source four-row ACCEPT bound", v.rowPointer.y)
	}
	_, _ = v.Handle(Actions{X: 319, Y: 249, Inside: true})
	if v.rowPointer.y != 180 {
		t.Fatal("outward distance accumulated or raw motion transformed twice", v.rowPointer)
	}
	for y := 248; y >= -100; y-- {
		_, _ = v.Handle(Actions{X: 319, Y: y, Inside: true})
	}
	if v.rowPointer.y != 67 || v.Hover != -1 {
		t.Fatal("heading must remain nonselectable at source top bound", v.rowPointer, v.Hover)
	}
	_, _ = v.Handle(Actions{X: 0, Y: -99, Inside: true})
	if v.rowPointer.y != 68 {
		t.Fatal("top reversal did not move immediately")
	}
	if v.Working != before {
		t.Fatal("cursor movement edited equipment")
	}
	// Focus/viewport resets establish a fresh raw anchor without moving the row.
	_, _ = v.Handle(Actions{Y: 180, Inside: true, PointerReset: true})
	if v.rowPointer.y != 68 {
		t.Fatal("focus reset applied an old raw delta")
	}
	_, _ = v.Handle(Actions{Y: 181, Inside: true})
	if v.rowPointer.y != 69 {
		t.Fatal("fresh anchor did not consume one raw pixel")
	}
}

func TestOrdnanceKeyboardRowsSelectorsAndLocalReturn(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	var input windowInput
	viewport := Viewport{Width: 320, Height: 200}
	step := func(b Buttons) {
		t.Helper()
		if _, err := v.Handle(input.ReadViewport(b, viewport)); err != nil {
			t.Fatal(err)
		}
	}
	first := v.Hover
	step(Buttons{X: 10, Y: 100, Inside: true, MenuBackTab: true})
	if v.Choices[v.Hover].Action != "back" {
		t.Fatal("ShiftTab should wrap past the heading to ACCEPT")
	}
	selected := v.Hover
	step(Buttons{X: 300, Y: 100, Inside: true, MenuBackTab: true})
	if v.Hover != selected {
		t.Fatal("held key or stationary raw pointer changed selection")
	}
	step(Buttons{X: 300, Y: 100, Inside: true})
	step(Buttons{X: 300, Y: 100, Inside: true, MenuTab: true})
	if v.Hover != first {
		t.Fatal("Tab did not wrap to first selectable row")
	}
	step(Buttons{X: 300, Y: 100, Inside: true, Enter: true})
	if len(v.stack) != 1 {
		t.Fatal("Selector2 must retain its source no-action route")
	}
	step(Buttons{X: 300, Y: 100, Inside: true})
	step(Buttons{X: 300, Y: 100, Inside: true, Space: true})
	if len(v.stack) != 2 || v.stack[0].Selected != first {
		t.Fatal("Space did not activate the visibly selected row")
	}
	_, _ = v.Activate(ordnanceChoice(t, v, "back", -1))
	if len(v.stack) != 1 || v.Hover != first {
		t.Fatal("nested return lost parent row")
	}
	for i, c := range v.Choices {
		if c.Action != "heading" && v.rowHit(v.rowRect(i)[1]) != i {
			t.Fatal("row hit mismatch", i)
		}
	}
}

func TestOrdnanceRowPointerUsesSourceTemplateEight(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	source := make([]byte, 64000)
	source[128], source[129], source[320+128] = 41, 5, 41
	source[0] = 99 // ordinary pointer must never leak into this composition.
	frame := bytes.Repeat([]byte{7}, 64000)
	y := v.rowPointer.y
	v.drawRowPointer(frame, source, 0)
	if frame[y*320+229] != 41 || frame[y*320+230] != 5 || frame[(y+1)*320+229] != 41 || frame[y*320+231] != 7 {
		t.Fatal("template8/zero-transparent source composition changed")
	}
	before := v.rowPointer
	v.drawRowPointer(frame, source, 0)
	if v.rowPointer != before {
		t.Fatal("drawing mutated pointer input state")
	}
}

func TestOrdnanceDirectionalClampHomeEndAndDeniedRows(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankWOC)
	_, _ = v.Handle(Actions{Y: 100, Inside: true})
	first := v.Hover
	_, _ = v.Handle(Actions{Y: 100, Inside: true, Up: true})
	if v.Hover != first {
		t.Fatal("directional Up incorrectly wrapped")
	}
	_, _ = v.Handle(Actions{Y: 100, Inside: true, MenuEnd: true})
	last := v.Hover
	if v.Choices[last].Action != "back" {
		t.Fatal("End did not select last row")
	}
	_, _ = v.Handle(Actions{Y: 100, Inside: true, Down: true})
	if v.Hover != last {
		t.Fatal("directional Down incorrectly wrapped")
	}
	_, _ = v.Handle(Actions{Y: 100, Inside: true, MenuHome: true})
	if v.Hover != first {
		t.Fatal("Home did not select first actionable row")
	}
	_, _ = v.Activate(ordnanceChoice(t, v, "aircraft", -1))
	before := v.Working
	_, _ = v.Handle(Actions{Y: 100, Inside: true, Down: true})
	if v.Choices[v.Hover].Enabled || v.Choices[v.Hover].Value != 1 {
		t.Fatal("rank-denied row must remain navigable")
	}
	_, _ = v.Handle(Actions{Y: 100, Inside: true, Selector1: true})
	if v.Message != "" || v.Working != before {
		t.Fatal("denied activation changed aircraft or duplicated upper-panel feedback in footer")
	}
}

func TestOrdnanceRowSelectorOriginalOperands(t *testing.T) {
	raw, err := os.ReadFile(testfixture.Path("installation/GS.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	// Named overlay2 original1d50, file33ae0+S; resident A=file+e000.
	for at, text := range map[int]string{
		0x33ae0 + 0x1589:        "ff36ac9b6a01ff36b09bff36ae9b9a3000f40e",
		0x33ae0 + 0x15cc:        "6a089a8209e61483c4026a006a006a089a9409e614",
		0x33ae0 + 0x1a82:        "b8b2008946d88946e0b804008946da8946e4b801008946e28946e6",
		0x257e2 - 0xe000 + 0x0a: "8b4606a37095",
	} {
		want, e := hex.DecodeString(text)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(raw[at:at+len(want)], want) {
			t.Fatalf("unsupported original selector operand atfile%05x", at)
		}
	}
}
