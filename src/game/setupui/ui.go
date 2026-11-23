// Package setupui is terminal-only. The game imports configuration, not this UI.
package setupui

import (
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/managedassets"
	"gunship2000-assets/game/state"
	"gunship2000-assets/internal/branding"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const Title = branding.ProductTitle + " Setup Utility by _provector"

type Setting struct {
	ID      string
	Kind    string
	Choices []string
	Get     func(*configuration.Config) string
	Set     func(*configuration.Config, string) error
}
type UI struct {
	Location          *configuration.Location
	LocationOverride  string
	pendingLocation   string
	writers           sync.WaitGroup
	controllers       *controllerUI
	ControllerBackend func() (controllers.Backend, error)
	App               *tview.Application
	Doc               *configuration.Document
	Help              *tview.TextView
	helpPanel         *helpPanel
	Footer            *tview.TextView
	Menu              *tview.List
	Table             *tview.Table
	Pages             *tview.Pages
	Main              *tview.Flex
	mainIDs           []string
	menu              string
	focus             map[string]int
	settings          []Setting
	presetButtons     *tview.Form
	popup             bool
	busy              bool
	Rebuild           func(func(string)) error
	Saved             bool
}

// tcell resolves named RGB colors to indexed colors on non-truecolor terminals.
var blue = tcell.ColorBlue.TrueColor()
var red = tcell.ColorMaroon.TrueColor()
var yellow = tcell.ColorYellow.TrueColor()
var white = tcell.ColorWhite.TrueColor()
var amber = tcell.NewRGBColor(160, 96, 0)

func New(d *configuration.Document) *UI {
	d.Config.Original = managedassets.OriginalForSetup(d.InstallDirectory(), d.Config.Original)
	u := &UI{App: tview.NewApplication(), Doc: d, pendingLocation: filepath.Dir(d.Path), focus: map[string]int{}}
	u.Help = tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	u.Help.SetBackgroundColor(blue)
	u.helpPanel = newHelpPanel(u.Help)
	u.Help.SetTextColor(white)
	u.Menu = tview.NewList().ShowSecondaryText(false).SetMainTextStyle(tcell.StyleDefault.Foreground(yellow).Background(red).Bold(true)).SetSelectedStyle(tcell.StyleDefault.Foreground(yellow).Background(blue).Bold(true)).SetHighlightFullLine(true)
	u.Menu.SetBorder(true).SetBackgroundColor(red)
	u.Table = tview.NewTable().SetSelectable(true, false).SetSelectedStyle(tcell.StyleDefault.Background(blue).Foreground(yellow).Bold(true))
	u.Table.SetBorder(true).SetBackgroundColor(red)
	u.Footer = tview.NewTextView().SetText("↑↓ Select  ←→ Adjust  Enter OK  Bksp Back  Esc Cancel  D - Debug Menu").SetTextColor(yellow)
	u.Footer.SetBackgroundColor(blue)
	u.Main = tview.NewFlex().SetDirection(tview.FlexRow)
	u.Pages = tview.NewPages().AddPage("main", u.Main, true, true)
	u.App.SetRoot(u.Pages, true).EnableMouse(false)
	u.App.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		_, h := s.Size()
		u.Main.ResizeItem(u.helpPanel, min(10, max(2, h-5)), 0)
		return false
	})
	u.App.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if buttons := u.presetButtons; buttons != nil && event != nil {
			for i := 0; i < buttons.GetButtonCount(); i++ {
				if buttons.GetButton(i).InRect(event.Position()) {
					return event, action
				}
			}
		}
		return nil, action
	})
	u.App.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyRune && e.Modifiers()&tcell.ModAlt != 0 && e.Modifiers()&tcell.ModCtrl == 0 && (e.Rune() == 'd' || e.Rune() == 'D') {
			u.RevealDebug()
			return nil
		}
		if v, handled := u.controllerInput(e); handled {
			return v
		}
		if e.Key() == tcell.KeyCtrlC {
			if !u.busy {
				u.App.Stop()
			}
			return nil
		}
		back := e.Key() == tcell.KeyBackspace || e.Key() == tcell.KeyBackspace2
		if u.busy && (back || e.Key() == tcell.KeyEscape) {
			return nil
		}
		if back && u.popup {
			if _, editing := u.App.GetFocus().(*tview.InputField); editing {
				return e
			}
			u.closePopup()
			return nil
		}
		if e.Key() == tcell.KeyEscape && u.popup {
			u.closePopup()
			return nil
		}
		if back || e.Key() == tcell.KeyEscape {
			if u.menu != "" {
				u.back()
			} else if !back {
				u.App.Stop()
			}
			return nil
		}
		return e
	})
	// The home list owns plain D. Editors, pickers and other menus keep
	// their regular rune handling.
	u.Menu.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyRune && e.Modifiers()&^tcell.ModShift == 0 && (e.Rune() == 'd' || e.Rune() == 'D') {
			if i := slices.Index(u.mainIDs, "debug"); i >= 0 {
				u.mainIDs = slices.Delete(u.mainIDs, i, i+1)
				u.Menu.RemoveItem(i)
			} else {
				u.RevealDebug()
			}
			return nil
		}
		return e
	})
	u.Menu.SetChangedFunc(func(i int, _, _ string, _ rune) {
		u.showHelp(u.mainIDs[i])
	})
	u.Table.SetSelectionChangedFunc(func(row, col int) {
		if row < len(u.settings) {
			u.focus[u.menu] = row
			u.showHelp(u.settings[row].ID)
		}
	})
	u.Table.SetSelectedFunc(func(row, col int) { u.activate(row) })
	u.Table.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		r, _ := u.Table.GetSelection()
		if r < len(u.settings) && u.settings[r].Kind == "level" {
			n, _ := strconv.Atoi(u.settings[r].Get(&u.Doc.Config))
			delta := 0
			switch e.Key() {
			case tcell.KeyLeft:
				delta = -1
			case tcell.KeyRight:
				delta = 1
			case tcell.KeyPgDn:
				delta = -10
			case tcell.KeyPgUp:
				delta = 10
			case tcell.KeyHome:
				delta = -100
			case tcell.KeyEnd:
				delta = 100
			}
			if delta != 0 {
				n = max(0, min(100, n+delta))
				u.change(r, strconv.Itoa(n))
				return nil
			}
		}
		return e
	})

	u.initControllers()
	u.mainIDs = []string{"graphics", "sound", "controls", "settings", "rebuild", "discard", "save"}
	for i, id := range u.mainIDs {
		u.insertMainItem(i, id)
	}
	u.home()
	return u
}
func (u *UI) text(id string) configuration.Text {
	t, ok := u.Doc.Config.UI[id]
	if ok {
		return t
	}
	if t, ok = configuration.DefaultText()[id]; ok {
		return t
	}
	return configuration.Text{Label: id, Tooltip: id}
}
func (u *UI) showHelp(id string) {
	u.helpPanel.buttonPreviewAxis = ""
	t := u.text(id)
	s := t.Tooltip
	if id == "original" {
		s += "\nLocal fallback: " + configuration.Relative(u.Doc.InstallDirectory(), configuration.DefaultOriginal)
	}
	if id == "saves_location" {
		s += "\n" + u.pendingLocation
		if u.LocationOverride != "" {
			s += " (set by " + u.LocationOverride + ")"
		}
	}
	if id == "saves" {
		s += "\nUser data: " + filepath.Dir(u.Doc.Path)
	}
	if id == "log_name" {
		s += "\nPath: " + configuration.LogPath(filepath.Dir(u.Doc.Path), u.Doc.Config.Logging)
	}
	u.helpPanel.controls = u.menu == "controls"

	for _, setting := range u.settings {
		if u.menu != "" && setting.ID == id {
			value := ""
			if setting.Get != nil {
				value = u.displayValue(setting, &u.Doc.Config)
			} else if id == "joystick" {
				value = u.controllerName()
			}
			label := t.Label
			if value != "" {
				label += ": " + value
			}
			s = label + "\n" + s
			break
		}
	}
	if id == "collective_mode" || id == "collective_source_override" {
		if p, ok := u.controllers.session.Profiles[u.controllers.session.Selected]; ok {
			source, reason := controllers.CollectiveSource(p)
			if id == "collective_mode" {
				s = collectiveHelp(controllers.CollectiveMode(p))
			} else {
				detail := "Using " + source + " (automatic)"
				if p.CollectiveSource != "" {
					detail = "Using " + source + " (your selection)"
				} else if strings.HasPrefix(reason, "unknown mechanics") {
					detail = "Unknown controller type. Using centered stick."
				}
				s = t.Tooltip + "\n" + detail
			}
		}
	}
	u.Help.SetText(tview.Escape(s))
}
func (u *UI) insertMainItem(index int, id string) {
	label := u.text(id).Label
	if id == "graphics" || id == "sound" || id == "controls" || id == "settings" || id == "debug" {
		label = strings.ToUpper(label)
	}
	u.Menu.InsertItem(index, tview.Escape(label), "", 0, func() {
		switch id {
		case "save":
			u.saveAndExit(false)
		case "discard":
			u.App.Stop()
		case "rebuild":
			u.rebuild()
		default:
			u.open(id)
		}
	})
}

// RevealDebug inserts one session-local row without rebuilding the current
// page. The list keeps the selected action even when its row moves.
func (u *UI) saveAndExit(confirmed bool) {
	var err error
	if u.pendingLocation != filepath.Dir(u.Doc.Path) {
		if u.Location == nil || u.LocationOverride != "" {
			u.message("Saves Location is set by the launch options.")
			return
		}
		err = u.Location.SwitchData(u.pendingLocation, confirmed, u.saveSession, state.ValidateStoredRoster)
	} else {
		err = u.saveSession()
	}
	if errors.Is(err, configuration.ErrExistingLocation) {
		m := tview.NewModal().SetText(tview.Escape("Use the existing careers and settings in " + u.pendingLocation + " on the next launch? Changes made here are saved to the current store.")).AddButtons([]string{"Cancel", "Switch"}).SetDoneFunc(func(i int, _ string) {
			u.closePopup()
			u.App.SetFocus(u.Menu)
			if i == 1 {
				u.saveAndExit(true)
			}
		})
		m.SetBackgroundColor(amber)
		u.overlay(m, 74, 12)
		return
	}
	if err != nil {
		u.message(err.Error())
		return
	}
	u.Saved = true
	u.App.Stop()
}
func (u *UI) RevealDebug() {
	if slices.Contains(u.mainIDs, "debug") {
		return
	}
	i := slices.Index(u.mainIDs, "rebuild")
	u.mainIDs = slices.Insert(u.mainIDs, i, "debug")
	u.insertMainItem(i, "debug")
}

func (u *UI) home() {
	u.menu = ""
	u.body(u.Menu)
	u.App.SetFocus(u.Menu)
	u.showHelp(u.currentID())
}
func (u *UI) open(id string) {
	remember := u.focus[id]
	u.menu = id
	u.settings = Catalogue(id)
	u.Table.Clear()
	u.Table.SetTitle("")
	if id == "settings" {
		u.Table.SetTitle(tview.Escape(strings.ToUpper(u.text(id).Label)))
	}
	for i := range u.settings {
		u.row(i)
	}
	u.body(u.Table)
	u.App.SetFocus(u.Table)
	r := min(remember, len(u.settings)-1)
	u.Table.Select(max(0, r), 0)
	if len(u.settings) > 0 {
		u.showHelp(u.settings[max(0, r)].ID)
	}
}
func (u *UI) displayValue(s Setting, config *configuration.Config) string {
	if s.Kind == "location" {
		return u.pendingLocation
	}
	value := s.Get(config)
	if s.Kind == "choice" || s.Kind == "cycle" {
		if choice, ok := u.text(s.ID).Choices[value]; ok {
			value = choice.Label
		}
	}
	return value
}

func (u *UI) row(i int) {
	s := u.settings[i]
	value := ""
	if s.Kind == "collective_mode" || s.Kind == "collective_source" {
		value = u.collectiveSettingValue(s.Kind)
	}
	if s.ID == "joystick" {
		value = u.controllerName()
	}
	if s.Get != nil || s.Kind == "location" {
		value = u.displayValue(s, &u.Doc.Config)
	}
	if s.Kind == "level" {
		n, _ := strconv.Atoi(value)
		value = volumeBar(n)
	}
	if s.Kind == "disabled" {
		value = "Unavailable"
	}
	label := u.text(s.ID).Label
	if s.Kind == "basic" || s.Kind == "controller_advanced" || s.Kind == "advanced" || s.Kind == "logging" || s.Kind == "device" {
		label = strings.ToUpper(label)
	}
	u.Table.SetCell(i, 0, tview.NewTableCell(tview.Escape(label)).SetTextColor(yellow).SetBackgroundColor(red).SetAttributes(tcell.AttrBold))
	if u.menuColumns() > 1 {
		u.Table.SetCell(i, 1, tview.NewTableCell(tview.Escape(value)).SetTextColor(white).SetBackgroundColor(red).SetAttributes(tcell.AttrBold))
	}
}
func (u *UI) change(row int, value string) {
	s := u.settings[row]
	c := u.Doc.Config
	if s.Set == nil {
		return
	}
	if e := s.Set(&c, value); e != nil {
		u.message(e.Error())
		return
	}
	if e := c.Validate(); e != nil {
		u.message(e.Error())
		return
	}
	u.Doc.Config = c
	for i := range u.settings {
		u.row(i)
	}
	u.showHelp(s.ID)
}
func (u *UI) activate(row int) {
	if row >= 0 && row < len(u.settings) {
		switch u.settings[row].Kind {
		case "collective_mode", "collective_source":
			u.chooseCollective(u.settings[row].Kind == "collective_source")
			return
		case "reset_bindings":
			u.resetBindings()
			return
		case "device":
			u.deviceChoices()
			return
		case "basic":
			u.openControllerEditor(false)
			return
		case "controller_advanced":
			u.openControllerEditor(true)
			return
		}
	}
	if row < 0 || row >= len(u.settings) {
		return
	}
	s := u.settings[row]
	switch s.Kind {
	case "preset":
		u.confirmGraphicsPreset(s.ID)
	case "toggle":
		u.change(row, "")
	case "choice":
		u.choose(row)
	case "cycle":
		for i, k := range s.Choices {
			if s.Get(&u.Doc.Config) == k {
				u.change(row, s.Choices[(i+1)%len(s.Choices)])
				break
			}
		}
	case "location":
		if u.LocationOverride != "" {
			u.message("Saves Location is fixed by " + u.LocationOverride + " for this launch.")
			return
		}
		u.input(row)
	case "level", "input":
		u.input(row)
	case "disabled":
		u.message(u.text(s.ID).Tooltip)
	case "back":
		u.back()
	case "advanced":
		u.open("advanced")
	case "logging":
		u.open("logging")
	case "rebuild":
		u.rebuild()
	case "commands":
		u.commands()
	}
}
func (u *UI) overlay(p tview.Primitive, w, h int) {
	u.popup = true
	center := tview.NewFlex().AddItem(nil, 0, 1, false).AddItem(tview.NewFlex().SetDirection(tview.FlexRow).AddItem(nil, 0, 1, false).AddItem(p, h, 0, true).AddItem(nil, 0, 1, false), w, 0, true).AddItem(nil, 0, 1, false)
	u.Pages.AddPage("popup", center, true, true)
	u.App.SetFocus(p)
}
func (u *UI) closePopup() {
	if u.presetButtons != nil {
		u.App.EnableMouse(false)
		u.presetButtons = nil
	}
	if u.controllers != nil {
		u.controllers.devicePicker = nil
	}
	u.Pages.RemovePage("popup")
	u.popup = false
	if u.menu == "" {
		u.App.SetFocus(u.Menu)
	} else {
		u.App.SetFocus(u.Table)
	}
	u.showHelp(u.currentID())
}
func (u *UI) choose(row int) {
	s := u.settings[row]
	var values []pickerValue
	for _, key := range s.Choices {
		key := key
		t, ok := u.text(s.ID).Choices[key]
		if !ok {
			t.Label = key
		}
		values = append(values, pickerValue{Key: key, Label: t.Label, Accept: func() { u.closePopup(); u.change(row, key) }})
	}
	list := valuePicker(u.text(s.ID).Label, values, s.Get(&u.Doc.Config))

	list.SetChangedFunc(func(i int, _, _ string, _ rune) {
		if i >= 0 && i < len(s.Choices) {
			q := u.text(s.ID).Choices[s.Choices[i]]
			u.Help.SetText(tview.Escape(u.text(s.ID).Tooltip + "\n" + q.Tooltip))
		}
	})
	list.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			u.closePopup()
			u.showHelp(s.ID)
			return nil
		}
		return e
	})
	u.overlay(list, 58, min(17, len(s.Choices)+2))
}
func (u *UI) input(row int) {
	s := u.settings[row]
	f := tview.NewForm()
	f.SetBackgroundColor(amber)
	f.SetLabelColor(yellow).SetFieldTextColor(white).SetFieldBackgroundColor(blue).SetButtonTextColor(yellow).SetButtonBackgroundColor(blue)
	f.SetBorder(true).SetTitle(tview.Escape(u.text(s.ID).Label))
	f.AddInputField("Value", u.displayValue(s, &u.Doc.Config), 32, nil, nil)
	f.AddButton("Apply", func() {
		v := f.GetFormItem(0).(*tview.InputField).GetText()
		u.closePopup()
		if s.Kind == "location" {
			if !filepath.IsAbs(v) {
				u.message("Enter an absolute path to a player-data folder.")
				return
			}
			u.pendingLocation = filepath.Clean(v)
			u.row(row)
			u.showHelp(s.ID)
			return
		}
		u.change(row, v)
	})
	f.AddButton("Cancel", u.closePopup)
	f.SetCancelFunc(u.closePopup)
	buttonArrows(f)
	u.overlay(f, 64, 9)
}
func (u *UI) message(text string) {
	m := tview.NewModal().SetText(tview.Escape(text)).AddButtons([]string{"OK"}).SetDoneFunc(func(_ int, _ string) {
		u.closePopup()
		if u.menu == "" {
			u.App.SetFocus(u.Menu)
		}
	})
	m.SetBackgroundColor(amber)
	m.SetTextColor(white).SetButtonTextColor(yellow).SetButtonBackgroundColor(blue)
	u.overlay(m, 70, 12)
}
func (u *UI) Run() error {
	defer u.writers.Wait()
	u.startControllers()
	defer u.stopControllers()
	return u.App.Run()
}
func (u *UI) rebuild() { u.runRebuild() }

// Static equal-size, midpoint-aligned preview crosses; no hardware sampling.
const NeutralAxes = "        Pitch               Collective\n          ░                     ░\n          ░                     ░\n Roll ░░░░█░░░░         Yaw ░░░░█░░░░\n          ░                     ░\n          ░                     ░"

func (u *UI) body(p tview.Primitive) {
	if table, ok := p.(*tview.Table); ok {
		p = fullWidthTable(table)
	}
	u.Main.Clear()
	u.Main.AddItem(u.helpPanel, 10, 0, false).AddItem(p, 0, 1, true).AddItem(u.Footer, 1, 0, false)
}
func (u *UI) back() {
	if u.menu == "advanced" || u.menu == "logging" {
		u.open("debug")
	} else if u.menu != "" {
		u.home()
	}
}
func (u *UI) currentID() string {
	if u.menu == "" {
		return u.mainIDs[u.Menu.GetCurrentItem()]
	}
	r, _ := u.Table.GetSelection()
	if r >= 0 && r < len(u.settings) {
		return u.settings[r].ID
	}
	return "back"
}
func volumeBar(n int) string {
	var b strings.Builder
	for i := 0; i < 20; i++ {
		q := n - i*5
		switch {
		case q >= 5:
			b.WriteRune('█')
		case q >= 4:
			b.WriteRune('▓')
		case q >= 2:
			b.WriteRune('▒')
		default:
			b.WriteRune('░')
		}
	}
	return fmt.Sprintf("%s %3d%%", b.String(), n)
}

func (u *UI) menuColumns() int {
	for _, s := range u.settings {
		if s.Get != nil || s.ID == "joystick" || s.Kind == "disabled" {
			return 2
		}
	}
	return 1
}
