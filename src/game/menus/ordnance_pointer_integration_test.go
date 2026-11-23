package menus

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOrdnanceRowSelectorActualControllerRoundTrip(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	if err := c.openOutfit(); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOrdnance {
		t.Fatal(c.Screen)
	}
	v := c.Mission.Ordnance
	beforeSave := c.Store.State()
	beforeRows := append([][36]byte(nil), c.Mission.ReadinessRows...)
	beforeMission := c.Mission.Data
	beforeObjects := beforeMission.ObjectHash
	var input windowInput
	viewport := Viewport{Width: 320, Height: 200}
	step := func(b Buttons) {
		t.Helper()
		if err := c.Handle(input.ReadViewport(b, viewport)); err != nil {
			t.Fatal(err)
		}
	}
	baseline := Buttons{X: 300, Y: 100, Inside: true}
	step(baseline)
	first, y := v.Hover, v.rowPointer.y
	left := baseline
	left.LeftMouse = true
	step(left)
	if len(v.stack) != 2 {
		t.Fatal("fixed-column click did not activate current row")
	}
	step(baseline)
	up := baseline
	up.MenuBackTab = true
	step(up)
	if v.Choices[v.Hover].Action != "back" {
		t.Fatal("nested ShiftTab wrap")
	}
	step(baseline)
	space := baseline
	space.Space = true
	step(space)
	if len(v.stack) != 1 || v.Hover != first || v.rowPointer.y != y {
		t.Fatal("nested return did not restore selected row")
	}
	step(baseline)
	right := baseline
	right.RightMouse = true
	step(right)
	if len(v.stack) != 1 {
		t.Fatal("Selector2 unexpectedly activated ordnance")
	}
	step(baseline)
	quit := baseline
	quit.AltQ = true
	step(quit)
	if c.Screen != ScreenQuit {
		t.Fatal(c.Screen)
	}
	cancel := baseline
	cancel.Escape = true
	step(cancel)
	if c.Screen != ScreenOrdnance || v.rowPointer.y != y {
		t.Fatal("quit cancellation lost row")
	}
	step(baseline)
	if v.rowPointer.y != y {
		t.Fatal("modal resume applied stale pointer delta")
	}
	frame := c.Frame(true)
	if !bytes.Equal(frame, c.Frame(true)) {
		t.Fatal("drawing changed row/description")
	}
	if output := os.Getenv("GS2000_J_ORDNANCE_OUTPUT"); output != "" {
		if err := os.Mkdir(output, 0755); err != nil {
			t.Fatal(err)
		}
		image, err := c.RenderRGBA(true)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(output, "ordnance-row-selector.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, image); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		doc := map[string]any{"level": "Native controller composition with prepared source assets; not a connected-window/original runtime witness", "aircraft": v.Working[0], "scene": v.Mission.Scene, "hover": v.Hover, "cursor_x": ordnanceSelectorX, "cursor_y": v.rowPointer.y, "raw_x": c.pointerX, "raw_y": c.pointerY, "frame_bytes": len(frame), "object_hash": beforeObjects}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, "case.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	step(up)
	step(baseline)
	step(space)
	if c.Screen != ScreenBriefing {
		t.Fatal("ACCEPT did not return through normal guard", c.Screen, v.Message)
	}
	if c.Mission.Data != beforeMission || c.Mission.Data.ObjectHash != beforeObjects || !reflect.DeepEqual(beforeSave, c.Store.State()) || !reflect.DeepEqual(beforeRows, c.Mission.ReadinessRows) {
		t.Fatal("cursor/menu round trips changed mission, equipment or persistent state")
	}
	if c.Office.PointerX != baseline.X || c.Office.PointerY != baseline.Y {
		t.Fatal("other screen did not retain ordinary free pointer coordinates")
	}
}
