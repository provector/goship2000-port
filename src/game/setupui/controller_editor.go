package setupui

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/controllers"
	"strings"
	"time"
)

func (u *UI) openControllerEditor(advanced bool) {
	c := u.controllers
	if c.err != nil {
		u.message(c.err.Error())
		return
	}
	p, ok := c.session.Profiles[c.session.Selected]
	if !ok {
		u.message("Select a controller first. <None> keeps saved profiles.")
		return
	}
	clone := controllers.Clone(controllers.Config{Version: 1, Selected: p.ID, Profiles: map[string]controllers.Profile{p.ID: p}})
	q := clone.Profiles[p.ID]
	c.draft = &q
	c.draftSwap = u.Doc.Config.Runtime.SwapYawRollAxis
	c.capture = nil
	c.wizard = false
	if advanced {
		c.screen = "advanced"
		u.controllerActions()
	} else {
		c.screen = "basic"
		u.basicTable()
	}
}
func (u *UI) controllerTable(title string) *tview.Table {
	t := tview.NewTable().SetSelectable(true, false).SetSelectedStyle(tcell.StyleDefault.Foreground(yellow).Background(blue).Bold(true))
	t.SetBorder(true).SetTitle(strings.ToUpper(title)).SetBackgroundColor(red)
	u.controllers.table = t
	u.body(t)
	u.App.SetFocus(t)
	return t
}
func cc(text string, selectable bool) *tview.TableCell {
	return tview.NewTableCell(tview.Escape(text)).SetSelectable(selectable).SetTextColor(yellow).SetBackgroundColor(red).SetAttributes(tcell.AttrBold)
}
func (u *UI) bindingText(id string) string {
	b, conflict := controllers.EffectiveBinding(*u.controllers.draft, id)
	if conflict {
		return "Legacy conflict: Enter resolves"
	}
	if b.Kind == "" {
		return "<Unassigned>"
	}
	return b.String()
}
func basicLabel(id string) string {
	switch id {
	case "menu.selector1":
		return "Selector #1"
	case "menu.selector2":
		return "Selector #2"
	case "weapon.weapon":
		return "Switch weapon"
	}
	return controllers.Label(id)
}

const (
	basicBack         = 2 // Two fixed header rows precede the selectable actions.
	basicWizard       = basicBack + 1
	basicFirstBinding = basicWizard + 1
	basicSwap         = basicFirstBinding + 3 // Immediately after Collective.
)

func basicIDAt(row int) (string, bool) {
	if row == basicSwap {
		return "", false
	}
	if row > basicSwap {
		row--
	}
	if row < basicFirstBinding || row >= basicFirstBinding+len(controllers.Basic) {
		return "", false
	}
	return controllers.Basic[row-basicFirstBinding], true
}
func (u *UI) basicHelp(row int) {
	if row == basicSwap {
		u.controllerHelp(u.Doc.Config.UI["swap_yaw_roll_axis"].Tooltip)
		return
	}
	if id, ok := basicIDAt(row); ok {
		a, _ := controllers.MappingAction(id)
		keys := "Enter: bind"
		if a.Axis {
			keys = "Enter: bind   Space: invert"
		}
		u.controllerHelp(basicLabel(id) + " | " + keys + "\n" + u.bindingText(id) + " | " + a.Keyboard)
		return
	}
	switch row {
	case basicBack:
		u.controllerHelp("Return to the controller menu and discard edits made on this screen.")
	case basicWizard:
		u.controllerHelp("Assign seven controls in order. Esc skips a control and keeps its binding.")
	default:
		u.controllerHelp("Keep these bindings in SETUP. Choose Save & Exit on the main menu to use them in the game.")
	}
}
func (u *UI) basicTable() {
	u.helpPanel.buttonPreviewAxis = ""
	c := u.controllers
	t := u.controllerTable("Basic Controls - draft")
	for r := 0; r <= 12; r++ {
		for col := 0; col < 3; col++ {
			t.SetCell(r, col, cc("", false))
		}
	}
	namedColumns(t, []string{"Action", "Controller binding", "Invert axis"})
	t.SetCell(basicBack, 0, cc("<-- Back", true))
	t.SetCell(basicWizard, 0, cc("<Run Wizard>", true))
	for i, id := range controllers.Basic {
		row := i + basicFirstBinding
		if row >= basicSwap {
			row++
		}
		a, _ := controllers.MappingAction(id)
		b, _ := controllers.EffectiveBinding(*c.draft, id)
		t.SetCell(row, 0, cc(basicLabel(id), true))
		t.SetCell(row, 1, cc(u.bindingText(id), true))
		if a.Axis {
			v := "Off"
			if b.Invert {
				v = "On"
			}
			t.SetCell(row, 2, cc(v, true))
		}
	}
	t.SetCell(basicSwap, 0, cc(u.Doc.Config.UI["swap_yaw_roll_axis"].Label, true))
	swapText := "Off"
	if c.draftSwap {
		swapText = "On"
	}
	t.SetCell(basicSwap, 1, cc(swapText, true))
	t.SetCell(12, 0, cc("<Save>", true))
	t.SetFixed(2, 0)
	t.SetSelectionChangedFunc(func(row, col int) { u.basicHelp(row) })
	t.SetSelectedFunc(func(row, col int) {
		if id, ok := basicIDAt(row); ok {
			c.returnRow, c.returnCol = row, col
			c.returnOffset, _ = t.GetOffset()
			c.wizard = false
			u.beginCapture(id)
			return
		}
		switch row {
		case basicBack:
			u.finishControllerEditor(false)
		case basicSwap:
			c.draftSwap = !c.draftSwap
			value := "Off"
			if c.draftSwap {
				value = "On"
			}
			t.GetCell(basicSwap, 1).SetText(value)
			u.basicHelp(basicSwap)
		case basicWizard:
			c.returnRow, c.returnCol = row, 0
			c.returnOffset, _ = t.GetOffset()
			c.wizard = true
			c.step = 0
			u.beginCapture(controllers.Basic[0])
		case 12:
			u.finishControllerEditor(true)
		}
	})
	t.Select(basicBack, 0)
}
func (u *UI) controllerActions() {
	c := u.controllers
	c.screen = "advanced"
	t := u.controllerTable("Advanced Controls - draft")
	namedColumns(t, []string{"Action", "Default keyboard", "Controller binding"})
	for col := 0; col < 3; col++ {
		t.SetCell(2, col, cc("", false))
	}
	t.SetCell(2, 0, cc("<-- Back", true))
	c.actionRows = map[int]controllers.Action{}
	actions := controllers.AdvancedActions()
	row := 3
	categories := []string{"Flight"}
	seen := map[string]bool{"Flight": true}
	for _, a := range actions {
		if a.Category != "Deferred" && !seen[a.Category] {
			categories = append(categories, a.Category)
			seen[a.Category] = true
		}
	}
	categories = append(categories, "Deferred")
	add := func(a controllers.Action) {
		c.actionRows[row] = a
		live := a.Status != "deferred"
		label := a.Label
		if !live {
			label += " (unavailable)"
		}
		value := u.bindingText(a.ID)
		if !live {
			value = "Unavailable"
		}
		t.SetCell(row, 0, cc(label, live))
		t.SetCell(row, 1, cc(a.Keyboard, false))
		t.SetCell(row, 2, cc(value, live))
		if !live {
			for col := 0; col < 3; col++ {
				t.GetCell(row, col).SetTextColor(tcell.ColorSilver)
			}
		}
		row++
	}
	for _, category := range categories {
		label := "------- " + category + " -------"
		for col := 0; col < 3; col++ {
			t.SetCell(row, col, cc("", false))
		}
		t.SetCell(row, 0, cc(label, false).SetReference(categoryHeader(label)))
		row++
		if category == "Flight" {
			for _, axis := range controllers.Basic[:4] {
				found := false
				for _, a := range actions {
					if a.ID == axis {
						add(a)
						found = true
						break
					}
				}
				if !found {
					a, _ := controllers.MappingAction(axis)
					for col := 0; col < 3; col++ {
						t.SetCell(row, col, cc("", false))
					}
					t.SetCell(row, 0, cc(a.Label+" (Basic)", false))
					t.SetCell(row, 1, cc("—", false))
					row++
				}
				for _, d := range controllers.AxisDirections {
					if d.Axis == axis {
						a, _ := controllers.MappingAction(d.ID)
						add(a)
					}
				}
			}
		}
		for _, a := range actions {
			if a.Category != category || category == "Flight" && a.Axis {
				continue
			}
			add(a)
		}
	}

	c.saveRow = row
	for col := 0; col < 3; col++ {
		t.SetCell(row, col, cc("", false))
	}
	t.SetCell(row, 0, cc("<Save>", true))
	t.SetFixed(2, 0)
	t.SetSelectionChangedFunc(func(row, col int) {
		u.helpPanel.buttonPreviewAxis = ""
		if a, ok := c.actionRows[row]; ok {
			if d, direction := controllers.DirectionByID(a.ID); direction {
				u.helpPanel.buttonPreviewAxis = d.Axis
			}
			if col == 2 {
				u.controllerHelp(u.bindingText(a.ID) + "\n" + a.Label + " | Enter: bind")
			} else {
				u.controllerHelp(a.Label + "\nKeyboard: " + a.Keyboard)
			}
		} else if row == c.saveRow {
			u.controllerHelp("Keep these bindings in SETUP. Choose Save & Exit on the main menu to use them in the game.")
		} else {
			u.controllerHelp("Return to the controller menu and discard edits made on this screen.")
		}
	})
	t.SetSelectedFunc(func(row, col int) {
		if row == 2 {
			u.finishControllerEditor(false)
		} else if row == c.saveRow {
			u.finishControllerEditor(true)
		} else if a, ok := c.actionRows[row]; ok && a.Status != "deferred" {
			c.returnRow, c.returnCol = row, col
			c.returnOffset, _ = t.GetOffset()
			c.wizard = false
			u.beginCapture(a.ID)
		}
	})
	t.Select(2, 0)
}
func (u *UI) finishControllerEditor(save bool) {
	c := u.controllers
	parent := 2
	if c.screen == "advanced" {
		parent = 3
	}
	if save {
		u.Doc.Config.Runtime.SwapYawRollAxis = c.draftSwap
		c.session.Profiles[c.draft.ID] = *c.draft
		c.session.Version = controllers.RequiresVersion(c.session)
	}
	c.draft = nil
	c.screen = ""
	c.capture = nil
	c.rearm = nil
	c.wizard = false
	u.open("controls")
	u.Table.Select(parent, 0)
}
func (u *UI) restoreControllerTable() {
	c := u.controllers
	if c.screen == "basic" {
		u.basicTable()
	} else {
		u.controllerActions()
	}
	c.table.SetOffset(c.returnOffset, 0)
	c.table.Select(c.returnRow, c.returnCol)
}
func (u *UI) controllerOverlay(p tview.Primitive, width, height int) {
	u.popup = true
	panel := tview.NewFlex().AddItem(nil, 0, 1, false).AddItem(p, width, 0, true).AddItem(nil, 0, 1, false)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(nil, 10, 0, false).AddItem(panel, height, 0, true).AddItem(nil, 0, 1, false)
	u.Pages.AddPage("popup", layout, true, true)
	u.App.SetFocus(p)
}
func (u *UI) closeControllerPopup() {
	u.closePopup()
	if u.controllers.screen != "" {
		u.App.SetFocus(u.controllers.table)
	}
}
func (u *UI) beginCapture(id string) {
	c := u.controllers
	c.captureID = controllers.LogicalID(id)
	if _, conflict := controllers.EffectiveBinding(*c.draft, id); conflict {
		u.resolveLegacyCapture(id)
		return
	}
	u.startCapture(c.captureID)
}
func (u *UI) startCapture(id string) {
	c := u.controllers
	if !c.connected {
		u.controllerHelp("Reconnect the controller or select another device. Your bindings are saved.")
		return
	}
	c.captureID = id
	c.capture = controllers.NewCapture(id, c.snapshot.States[c.device.Instance])
	v := newCapturePanel(u)
	c.captureView = v
	v.SetText(tview.Escape(u.captureText()))
	u.controllerOverlay(v, 68, 12)
	u.controllerHelp("Release held buttons, then press a button or move an axis firmly. Space clears the binding. Esc keeps the old binding.")
}
func (u *UI) captureText() string {
	c := u.controllers
	step := ""
	if c.wizard {
		step = fmt.Sprintf("Wizard %d / %d\n", c.step+1, len(controllers.Basic))
	}
	prompt := "Return the used axis to its starting position / release controls to re-arm"
	if c.capture != nil {
		prompt = c.capture.Prompt
	}
	return step + controllers.Label(c.captureID) + "\n" + prompt + "\n\nEsc: " + map[bool]string{true: "skip row (keep old)", false: "cancel (keep old)"}[c.wizard] + "   Space: clear binding"
}
func (u *UI) acceptCaptured(b controllers.Binding) {
	c := u.controllers
	old := c.capture
	c.capture = nil
	prior, _ := controllers.EffectiveBinding(*c.draft, c.captureID)
	if b.Kind == "axis" || b.Kind == "pair" {
		b.Invert = prior.Invert
	}
	conflicts := controllers.Conflicts(*c.draft, c.captureID, b)
	apply := func() {
		for _, key := range conflicts {
			controllers.SetBinding(c.draft, key, controllers.Binding{})
		}
		controllers.SetBinding(c.draft, c.captureID, b)
		u.afterCapture(old)
	}
	if len(conflicts) == 0 {
		apply()
		return
	}
	c.pending = &b
	labels := make([]string, len(conflicts))
	for i, id := range conflicts {
		labels[i] = basicLabel(id)
	}
	m := tview.NewModal().SetText("Control also used here: " + strings.Join(labels, ", ") + ". Move it to this action?").AddButtons([]string{"Move binding", "Cancel"}).SetDoneFunc(func(i int, _ string) {
		c.pending = nil
		if i == 0 {
			apply()
		} else {
			u.afterCapture(nil)
		}
	})
	m.SetBackgroundColor(amber).SetTextColor(yellow).SetButtonBackgroundColor(blue)
	u.controllerOverlay(m, 70, 8)
}
func (u *UI) afterCapture(used *controllers.Capture) {
	c := u.controllers
	u.closeControllerPopup()
	c.capture = nil
	c.pending = nil
	if c.wizard {
		c.step++
		if c.step < len(controllers.Basic) {
			c.captureID = controllers.Basic[c.step]
			u.beginCapture(c.captureID)
			if used != nil && used.Binding.Kind == "axis" && c.capture != nil && used.Binding.Axis < len(used.Baseline.Axes) {
				c.capture.SuppressAxis(used.Binding.Axis, used.Baseline.Axes[used.Binding.Axis])
			}
			return
		}
		c.wizard = false
	}
	u.restoreControllerTable()
}
func (u *UI) controllerInput(e *tcell.EventKey) (*tcell.EventKey, bool) {
	c := u.controllers
	if c == nil {
		return e, false
	}
	if e.Key() == controllerWake {
		u.receiveControllers()
		return nil, true
	}
	if !c.wizard && !c.swallowTransition {
		c.keyUntil = time.Time{}
	}
	if e.Key() != c.lastKey || e.Rune() != c.lastRune {
		c.keyUntil = time.Time{}
		c.swallowTransition = false
	} else if time.Now().Before(c.keyUntil) {
		c.keyUntil = time.Now().Add(750 * time.Millisecond)
		return nil, true
	}
	if (c.capture != nil || c.rearm != nil) && c.pending == nil {
		escape := e.Key() == tcell.KeyEscape
		clear := e.Key() == tcell.KeyRune && e.Rune() == ' '
		if escape || clear {
			now := time.Now()

			c.swallowTransition = clear || c.wizard
			c.lastKey, c.lastRune = e.Key(), e.Rune()
			c.keyUntil = now.Add(750 * time.Millisecond)
			if clear {
				controllers.SetBinding(c.draft, c.captureID, controllers.Binding{})
			}
			c.capture = nil
			c.rearm = nil
			if c.wizard {
				u.afterCapture(nil)
			} else {
				u.closeControllerPopup()
				u.restoreControllerTable()
			}
		}
		return nil, true
	}
	if c.pending != nil && (e.Key() == tcell.KeyEscape || e.Key() == tcell.KeyBackspace || e.Key() == tcell.KeyBackspace2) {
		c.pending = nil
		u.afterCapture(nil)
		return nil, true
	}

	if c.screen == "basic" && !u.popup && e.Key() == tcell.KeyRune && e.Rune() == ' ' {
		row, _ := c.table.GetSelection()
		if id, ok := basicIDAt(row); ok && func() bool { a, _ := controllers.MappingAction(id); return a.Axis }() {
			b, _ := controllers.EffectiveBinding(*c.draft, id)
			b.Invert = !b.Invert
			controllers.SetBinding(c.draft, id, b)
			value := "Off"
			if b.Invert {
				value = "On"
			}
			c.table.GetCell(row, 2).SetText(value)
			u.basicHelp(row)
		}
		return nil, true
	}
	if c.screen != "" && !u.popup && (e.Key() == tcell.KeyEscape || e.Key() == tcell.KeyBackspace || e.Key() == tcell.KeyBackspace2) {
		u.finishControllerEditor(false)
		return nil, true
	}
	return e, false
}

func (u *UI) resolveLegacyCapture(id string) {
	c := u.controllers
	ids := controllers.BindingIDs(id)
	c.capture = nil
	c.pending = &controllers.Binding{}
	m := tview.NewModal().SetText("Choose a binding for both uses. Cancel keeps both.\nMenu: " + c.draft.Bindings[ids[0]].String() + "\nFlight: " + c.draft.Bindings[ids[1]].String()).AddButtons([]string{"Use menu", "Use flight", "Bind new", "Cancel"})
	m.SetBackgroundColor(amber).SetTextColor(yellow).SetButtonBackgroundColor(blue)
	m.SetDoneFunc(func(i int, _ string) {
		c.pending = nil
		u.closeControllerPopup()
		if i == 0 || i == 1 {
			u.acceptCaptured(c.draft.Bindings[ids[i]])
		} else if i == 2 {
			u.startCapture(controllers.LogicalID(id))
		} else {
			u.afterCapture(nil)
		}
	})
	u.controllerOverlay(m, 70, 10)
	u.controllerHelp("Menu and flight use different bindings. Choose which to keep or assign a new one. Esc keeps both.")
}

func collectiveSource(p *controllers.Profile) string {
	if p.CollectiveSource == "slider" {
		return "slider"
	}
	return "centered"
}
func collectiveSourceLabel(p *controllers.Profile) string {
	if collectiveSource(p) == "slider" {
		return "Slider"
	}
	return "Centered stick"
}
func collectiveModeLabel(mode string) string {
	switch mode {
	case "relative":
		return "Relative"
	case "relative_height":
		return "Relative Height"
	}
	return "Absolute"
}

func collectiveHelp(mode string) string {
	switch mode {
	case "relative":
		return "Relative: move the stick away from center to change thrust. With a slider, move it to change thrust and stop to hold."
	case "relative_height":
		return "Relative Height: center the control to hold height on Easy. Realistic or a critical failure uses manual relative thrust."
	default:
		return "Absolute: lever position sets thrust. Move through the current setting to take control after launch, Resume or keyboard use."
	}
}
