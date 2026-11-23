package menus

import (
	"gunship2000-assets/game/flightcontrol"
	"time"
)

// Buttons is device state; Actions contains edges consumed once by a screen.
// The latch survives screen transitions, so the opening press cannot activate
// the destination while the user is still holding it.
type Buttons struct {
	WingmanOrder                                      string
	ControllerTarget                                  bool    // Physical Selector2 is independent of keyboard Alt/Shift.
	ViewCommandShift                                  [9]bool // A mapped Shift+Fn belongs only to that view action.
	Live                                              *flightcontrol.LiveInput
	ReplayResume, ReplayDemo, ReplayMap               bool
	CompressionIncrease, CompressionReset             bool
	WeaponTrigger                                     bool // Held Selector1 only in the flight keyboard/device domain.
	WeaponTriggerPressed                              bool // Backend make event; may be released before the next poll.
	FlightKeys                                        [11]bool
	DefenseKeys                                       [4]bool
	RocketSalvoKeys                                   [3]bool // main-row1/2/4, not keypad or Selector2
	GPSOpen                                           bool
	OrdnanceNext                                      bool
	LastMessage, Jettison                             bool
	GPSKey                                            string
	WaypointKeys                                      [2]bool
	FlightPause, FlightFocusLost                      bool
	LeftMouse, RightMouse, Space, Enter, Escape, AltQ bool
	AltE                                              bool
	AltT                                              bool
	AltC                                              bool
	FunctionKeys                                      [9]bool
	SceneDirections                                   [4]bool
	PageUp, PageDown                                  bool
	ViewShift, AltS                                   bool
	MenuTab, MenuBackTab                              bool
	MenuHome, MenuEnd                                 bool
	Left, Right, Up, Down, Backspace, Delete          bool
	Close, ConfirmQuit                                bool
	AnyKeyPressed, AnyKeyDown                         bool
	Text                                              []rune
	X, Y                                              int
	Inside                                            bool
	PointerRelative                                   bool
	PointerDX, PointerDY                              int
	PointerEpoch                                      bool
	InputTime                                         time.Duration
}

type Actions struct {
	gpsSteps                              []int                    // Preflight-only Alt+Fn make events.
	WingmanOrder                          string                   `json:"-"`
	WingmanRecipient                      int                      `json:"-"`
	Selector1Down                         bool                     `json:",omitempty"`
	SelectorHoldValid                     bool                     `json:",omitempty"`
	ControllerTarget                      bool                     `json:"-"` // resolved into the existing logical target operation
	CollectiveShift, CollectiveShiftValid bool                     `json:"-"`
	Live                                  *flightcontrol.LiveInput `json:"-"` // device sampling only, recorded per application tick
	// Editor transport only: never extend a recorded flight boundary/schema.
	PointerMoved                                     bool `json:"-"`
	PointerLeftHeld                                  bool `json:"-"`
	PointerRightHeld                                 bool `json:"-"`
	PointerReleased                                  bool `json:"-"`
	PointerStateValid                                bool `json:"-"`
	ReplayResume, ReplayDemo, ReplayMap              bool
	CompressionIncrease, CompressionReset            bool
	WeaponTriggerHeld, WeaponTriggerValid            bool
	WeaponTriggerPressed                             bool
	FlightCommands                                   []int
	DefenseCommands                                  []int
	DefenseHeld                                      [2]bool // radar, IR; physical down state for BIOS-style make repeats
	DefenseHeldValid                                 bool
	RocketSalvo                                      int
	GPSOpen                                          bool
	OrdnanceNext                                     bool
	LastMessage, Jettison                            bool `json:"-"` // Recorded as R4 logical Aux operations, not host boundaries.
	GPSKey                                           string
	WaypointNext, WaypointPrevious                   bool
	FlightHeldKeys                                   [11]bool
	FlightHeldValid                                  bool
	InputTime                                        time.Duration
	FlightPause, FlightFocusLost                     bool
	Selector1, Selector2, Enter, Cancel, Quit, Close bool
	PointerPressed                                   bool
	Left, Right, Up, Down, Backspace                 bool
	Delete                                           bool `json:"-"`
	Text                                             []rune
	X, Y                                             int
	Inside                                           bool
	PointerReset                                     bool // viewport/focus anchor changed; never a game action
	PointerRelative                                  bool // DX/DY are direct normalized raw movement, not constrained coordinates
	PointerDX, PointerDY                             int
	AnyKey                                           bool
	Skip                                             bool
	ConfirmQuit                                      bool
	EndMission                                       bool
	NightVisionToggle                                bool
	SpeechToggle                                     bool
	ViewKey                                          int
	SceneDirections                                  [4]bool
	ViewCycle                                        int
	ViewShift, CycleArrangement                      bool
	MenuTab, MenuBackTab                             bool
	MenuHome, MenuEnd                                bool
}

type inputLatch struct {
	previous     Buttons
	suppressText bool
}

// Shared by the native window and controller tests, including focus/letterbox
// changes. Device edges and pointer anchors are resolved before screen handling.
type windowInput struct {
	inputLatch
	viewport Viewport
	active   bool
}

func (w *windowInput) ReadViewport(b Buttons, viewport Viewport) Actions {
	a := w.Read(b)
	a.PointerReset = b.PointerEpoch || viewport != w.viewport || b.Inside != w.active
	w.viewport, w.active = viewport, b.Inside
	return a
}

// Flight/GPS use the expansion's default keyboard selectors: Return is #1,
// Backspace is #2. Space has its own ordnance/GPS action, unlike menu selection.
func normalizeSceneSelectors(a Actions, leftPressed, rightPressed, keysAllowed bool) Actions {
	a.Backspace = a.Backspace && keysAllowed
	a.Selector1Down = (a.Selector1Down && keysAllowed) || a.PointerLeftHeld
	a.Selector1 = (a.Enter && keysAllowed) || (a.PointerPressed && leftPressed)
	a.Selector2 = a.Backspace || (a.PointerPressed && rightPressed) || a.ControllerTarget
	return a
}

func (l *inputLatch) SuppressTextUntilRelease() { l.suppressText = true }

func (l *inputLatch) Read(b Buttons) Actions {
	p := l.previous
	a := Actions{
		WingmanOrder: func() string {
			if b.WingmanOrder == p.WingmanOrder {
				return ""
			}
			return b.WingmanOrder
		}(),
		Selector1Down: b.LeftMouse || b.Enter, SelectorHoldValid: true,
		ControllerTarget: b.ControllerTarget && !(p.ControllerTarget || p.Backspace),
		CollectiveShift:  b.ViewShift, CollectiveShiftValid: b.Live != nil,
		Live:            b.Live,
		PointerLeftHeld: b.LeftMouse, PointerRightHeld: b.RightMouse,
		PointerReleased: (p.LeftMouse && !b.LeftMouse) || (p.RightMouse && !b.RightMouse), PointerStateValid: true,
		ReplayResume: b.ReplayResume && !p.ReplayResume, ReplayDemo: b.ReplayDemo && !p.ReplayDemo, ReplayMap: b.ReplayMap && !p.ReplayMap,
		WeaponTriggerHeld: b.WeaponTrigger, WeaponTriggerValid: true,
		WeaponTriggerPressed: b.WeaponTriggerPressed && (b.Live == nil || !p.WeaponTrigger) || (b.WeaponTrigger && !p.WeaponTrigger),
		OrdnanceNext:         b.OrdnanceNext && !p.OrdnanceNext,
		LastMessage:          b.LastMessage && !p.LastMessage,
		Jettison:             b.Jettison && !p.Jettison,
		GPSOpen:              b.GPSOpen && !p.GPSOpen, GPSKey: func() string {
			if b.GPSKey == p.GPSKey {
				return ""
			}
			return b.GPSKey
		}(), WaypointNext: b.WaypointKeys[0] && !p.WaypointKeys[0], WaypointPrevious: b.WaypointKeys[1] && !p.WaypointKeys[1],
		FlightHeldKeys: b.FlightKeys, FlightHeldValid: true, InputTime: b.InputTime,
		CompressionIncrease: b.CompressionIncrease && !p.CompressionIncrease, CompressionReset: b.CompressionReset && !p.CompressionReset,
		FlightPause: b.FlightPause && !p.FlightPause, FlightFocusLost: b.FlightFocusLost,
		ViewShift:      b.ViewShift,
		PointerPressed: (b.LeftMouse && !p.LeftMouse) || (b.RightMouse && !p.RightMouse),
		Selector1:      (b.LeftMouse && !p.LeftMouse) || (b.Space && !p.Space),
		Selector2:      (b.RightMouse && !p.RightMouse) || (b.Enter && !p.Enter),
		Enter:          b.Enter && !p.Enter, Cancel: b.Escape && !p.Escape, Quit: b.AltQ && !p.AltQ, Close: b.Close && !p.Close,
		Left: b.Left && !p.Left, Right: b.Right && !p.Right, Up: b.Up && !p.Up, Down: b.Down && !p.Down, Backspace: b.Backspace && !p.Backspace && (b.Live == nil || !p.ControllerTarget),
		Text: b.Text, X: b.X, Y: b.Y, Inside: b.Inside,
		PointerRelative: b.PointerRelative, PointerDX: b.PointerDX, PointerDY: b.PointerDY,
		AnyKey:            b.AnyKeyPressed,
		Skip:              b.Space && !p.Space,
		ConfirmQuit:       b.ConfirmQuit && !p.ConfirmQuit,
		EndMission:        b.AltE && !p.AltE,
		NightVisionToggle: b.AltT && !p.AltT,
		SpeechToggle:      b.AltC && !p.AltC,
		Delete:            b.Delete && !p.Delete,
		MenuTab:           b.MenuTab && !p.MenuTab, MenuBackTab: b.MenuBackTab && !p.MenuBackTab,
		MenuHome: b.MenuHome && !p.MenuHome, MenuEnd: b.MenuEnd && !p.MenuEnd,
	}
	for i, down := range b.DefenseKeys {
		if down && !p.DefenseKeys[i] {
			a.DefenseCommands = append(a.DefenseCommands, i)
		}
	}
	a.DefenseHeld, a.DefenseHeldValid = [2]bool{b.DefenseKeys[2], b.DefenseKeys[3]}, true
	for i, down := range b.RocketSalvoKeys {
		if down && !p.RocketSalvoKeys[i] {
			a.RocketSalvo = [3]int{1, 2, 4}[i]
		}
	}
	for i, down := range b.FlightKeys {
		if down && !p.FlightKeys[i] {
			key := i
			if b.ViewShift {
				key |= 0x100
			}
			a.FlightCommands = append(a.FlightCommands, key)
		}
	}
	for i, down := range b.FunctionKeys {
		if down && !p.FunctionKeys[i] {
			a.ViewKey = i + 1
			a.ViewShift = b.ViewShift || b.ViewCommandShift[i]
		}
	}
	a.CycleArrangement = b.AltS && !p.AltS
	a.SceneDirections = b.SceneDirections
	if b.PageUp && !p.PageUp {
		a.ViewCycle = 1
	}
	if b.PageDown && !p.PageDown {
		a.ViewCycle = -1
	}
	if l.suppressText {
		if b.AnyKeyDown || !b.AnyKeyPressed {
			a.Text = nil
		}
		if !b.AnyKeyDown {
			l.suppressText = false
		}
	}
	b.Text = nil
	l.previous = b
	return a
}
