//go:build !headless

package menus

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/internal/presentationstats"
	"runtime"
	"slices"
	"time"
)

// The desktop owns the modal. No flight pause, replay operation or input command
// is submitted. The opening pause state and interpolation snapshots stay intact.
type graphicsWindow struct {
	menu            *graphicsOptions
	consumed        map[int32]bool
	windowed        displayWindow
	mode            configuration.WindowMode
	placement       *windowPlacement
	pending         *displayWindow
	deadline        time.Time
	handoff         *displayWindow
	nativeWayland   bool
	desktop         *desktopFullscreen
	handoffDeadline time.Time
}

// Quarantine only the menu buttons that remain physically held on exit.
// Unrelated flight inputs can be sampled on the first live frame.
func (s *graphicsWindow) consumeHeldMenuKeys() {
	for _, key := range [...]int32{rl.KeyEnter, rl.KeyKpEnter, rl.KeyEscape, rl.KeyLeft, rl.KeyRight, rl.KeyDown, rl.KeyUp, rl.KeyO, rl.KeyL} {
		if rl.IsKeyDown(key) {
			if s.consumed == nil {
				s.consumed = make(map[int32]bool)
			}
			s.consumed[key] = true
		}
	}
}

// A mission point owns input through confirmation, reconstruction and the
// fade. Held keyboard controls must make a fresh press after that transition.
func (s *graphicsWindow) consumeHeldPointKeys() {
	for key := int32(rl.KeySpace); key <= rl.KeyRightSuper; key++ {
		if rl.IsKeyDown(key) {
			if s.consumed == nil {
				s.consumed = make(map[int32]bool)
			}
			s.consumed[key] = true
		}
	}
}

func (s *graphicsWindow) keyDown(key int32) bool {
	return !s.consumed[key] && rl.IsKeyDown(key)
}

func (s *graphicsWindow) keyPressed(key int32) bool { return !s.consumed[key] && rl.IsKeyPressed(key) }

// Fullscreen dimensions describe the monitor. Keep the client's last windowed
// rectangle separately so a temporary shortcut or cancelled preview can restore it.
func (s *graphicsWindow) currentWindow() displayWindow {
	if s.handoff != nil {
		return *s.handoff
	}
	if s.mode == configuration.Windowed && s.placement == nil {
		s.windowed = s.observedClient()
	}
	w := s.windowed
	w.mode = s.mode
	return w
}
func (s *graphicsWindow) setDisplayWindow(w displayWindow) {
	current := s.currentWindow()
	if s.mode == configuration.Windowed && s.placement == nil {
		s.desktop.rememberFrame()
	}
	s.placement = nil
	if current == w && s.handoff == nil && (w.mode != configuration.Windowed || s.observedClient() == w) {
		return
	}
	// Leave the old style before setting the return rectangle for the next.
	if s.handoff != nil {
		s.handoff = &w
		return
	}
	// Native Wayland exposes one compositor fullscreen state. Changing
	// styles there needs no detach/reattach, which would also discard
	// libdecor's windowed size during the asynchronous configure.
	if s.nativeWayland && s.mode != configuration.Windowed && w.mode != configuration.Windowed {
		s.mode = w.mode
		return
	}
	wasManaged, managed := s.desktop.manages(s.mode), s.desktop.manages(w.mode)
	if wasManaged {
		if managed && w.monitor == current.monitor {
			s.mode, s.windowed = w.mode, w
			return
		}
		s.desktop.request(false)
		s.handoff = &w
		s.handoffDeadline = time.Now().Add(time.Second)
		return
	}
	switch s.mode {
	case configuration.FullScreen:
		if rl.IsWindowFullscreen() {
			rl.ToggleFullscreen()
		}
	case configuration.Borderless:
		if s.nativeWayland {
			rl.ToggleFullscreen()
		} else if runtime.GOOS == "linux" {
			rl.ToggleBorderlessWindowed()
			// GLFW ignores decoration changes while attached to a monitor.
			// Raylib attempted to decorate before detaching. Reapply after.
			rl.SetWindowState(rl.FlagWindowUndecorated)
			rl.ClearWindowState(rl.FlagWindowUndecorated)
		} else {
			rl.ClearWindowState(rl.FlagWindowUndecorated | rl.FlagWindowTopmost)
		}
	}
	s.enterWindow(w)
}
func (s *graphicsWindow) enterWindow(w displayWindow) {
	if w.mode == configuration.Windowed || w.monitor != rl.GetCurrentMonitor() {
		rl.SetWindowSize(w.width, w.height)
		if !s.nativeWayland {
			rl.SetWindowPosition(w.x, w.y)
		}
	}
	s.mode = w.mode
	if w.mode == configuration.Windowed {
		s.placement = &windowPlacement{target: w, started: time.Now()}
	} else {
		s.windowed = w
	}
	if s.desktop.manages(w.mode) {
		s.desktop.request(true)
		return
	}
	switch w.mode {
	case configuration.FullScreen:
		rl.ToggleFullscreen()
	case configuration.Borderless:
		if s.nativeWayland {
			rl.ToggleFullscreen()
		} else if runtime.GOOS == "linux" {
			// Ask the compositor for fullscreen at the current desktop mode.
			// GLFW compares that mode before issuing any XRandR mode change.
			rl.ToggleBorderlessWindowed()
		} else {
			monitor := w.monitor
			if monitor < 0 || monitor >= rl.GetMonitorCount() {
				monitor = rl.GetCurrentMonitor()
			}
			pos := rl.GetMonitorPosition(monitor)
			rl.SetWindowState(rl.FlagWindowUndecorated | rl.FlagWindowTopmost)
			rl.SetWindowPosition(int(pos.X), int(pos.Y))
			rl.SetWindowSize(rl.GetMonitorWidth(monitor), rl.GetMonitorHeight(monitor))
		}
	}
}

func (s *graphicsWindow) matches(w displayWindow) bool {
	if s.handoff != nil || s.mode != w.mode {
		return false
	}
	actual := s.observedClient()
	if w.mode == configuration.Windowed {
		return !rl.IsWindowFullscreen() && actual.width == w.width && actual.height == w.height
	}
	if s.desktop.manages(w.mode) {
		if !s.desktop.fullscreen() {
			return false
		}
	} else if w.mode == configuration.FullScreen && !rl.IsWindowFullscreen() {
		return false
	}
	m := w.monitor
	if m < 0 || m >= rl.GetMonitorCount() {
		return false
	}
	pos := rl.GetMonitorPosition(m)
	return actual.width == rl.GetMonitorWidth(m) && actual.height == rl.GetMonitorHeight(m) && (s.nativeWayland || actual.x == int(pos.X) && actual.y == int(pos.Y))
}

func (s *graphicsWindow) input(o *WindowOptions, keys []int32, alt, closing bool, meter **presentationstats.Meter) (blocked bool, notice string) {
	if w := s.handoff; w != nil {
		if !s.desktop.fullscreen() {
			s.handoff = nil
			s.enterWindow(*w)
		} else if time.Now().After(s.handoffDeadline) {
			s.handoff, s.pending = nil, nil
			if s.menu != nil {
				s.menu.draft.window = s.currentWindow()
				s.menu.dirty = true
			}
			notice = "The desktop declined the window change."
		}
	}
	if s.placement != nil {
		if !s.settlePlacement() {
			notice = "The desktop could not restore the requested window placement."
		}
	}
	if runtime.GOOS != "linux" && s.mode == configuration.Borderless {
		// Cover the taskbar while playing; let other applications come forward on Alt+Tab.
		if focused, above := rl.IsWindowFocused(), rl.IsWindowState(rl.FlagWindowTopmost); focused != above {
			if focused {
				rl.SetWindowState(rl.FlagWindowTopmost)
			} else {
				rl.ClearWindowState(rl.FlagWindowTopmost)
			}
		}
	}
	if s.menu == nil {
		if !alt || !slices.Contains(keys, int32(rl.KeyO)) || !rl.IsWindowFocused() || closing {
			return false, notice
		}
		if _, p := graphicsScene(o.FrontEnd); p == nil {
			return false, notice
		}
		prefs := displayPreferences{o.Runtime, s.currentWindow(), o.DrawFPSCPU}
		s.menu = &graphicsOptions{draft: prefs, original: prefs, dirty: true}
		return true, ""
	}
	g := s.menu
	if s.pending != nil && s.placement == nil {
		if s.matches(*s.pending) {
			if s.mode == configuration.Windowed {
				g.draft.window = s.currentWindow()
			}
			s.pending = nil
		} else if s.handoff == nil && time.Now().After(s.deadline) {
			if s.mode == configuration.Windowed && s.pending.mode == configuration.Windowed {
				g.draft.window = s.currentWindow()
				g.dirty = true
				s.pending = nil
				notice = "Window size limited by the desktop."
			} else {
				s.setDisplayWindow(g.original.window)
				g.draft.window = g.original.window
				g.windowChosen = false
				s.pending = nil
				g.dirty = true
				notice = "Window change unavailable. Restoring the opening window."
			}
		}
	}
	// A fresh press takes precedence over a repeat from the opposite key.
	if g.row == 3 && rl.IsWindowFocused() && !slices.Contains(keys, rl.KeyLeft) && !slices.Contains(keys, rl.KeyRight) && rl.IsKeyDown(rl.KeyLeft) != rl.IsKeyDown(rl.KeyRight) {
		for _, key := range [...]int32{rl.KeyLeft, rl.KeyRight} {
			if rl.IsKeyDown(key) && rl.IsKeyPressedRepeat(key) {
				keys = append(keys, key)
			}
		}
	}
	// Several key makes may arrive in one poll. Advance the list for each
	// one, but only send the final preview in that frame to the desktop.
	windowChanged := false
	previewWindow := func() {
		if !windowChanged {
			return
		}
		w := g.draft.window
		if w.mode != configuration.Windowed {
			// A burst of arrows may have crossed a numerical row that was
			// never presented. Only a genuine client rectangle is a return point.
			returnTo := s.currentWindow()
			returnTo.mode = w.mode
			w, g.draft.window = returnTo, returnTo
		} else {
			current := s.currentWindow()
			w.x, w.y, w.monitor = current.x, current.y, rl.GetCurrentMonitor()
		}
		s.setDisplayWindow(w)
		if w.mode == configuration.Windowed && s.handoff == nil && s.placement == nil {
			s.placement = &windowPlacement{target: w, started: time.Now()}
		}
		s.pending = &w
		s.deadline = time.Now().Add(550 * time.Millisecond)
		windowChanged = false
	}
	for _, key := range keys {
		if closing {
			break
		}
		switch key {
		case rl.KeyUp:
			g.row = (g.row + 4) % 5
		case rl.KeyDown:
			g.row = (g.row + 1) % 5
		case rl.KeyRight, rl.KeyLeft:
			before := g.draft
			d := 1
			if key == rl.KeyLeft {
				d = -1
			}
			g.cycle(d)
			if before.runtime != g.draft.runtime {
				applyDisplay(o.FrontEnd, g.draft.runtime)
				if before.runtime.PresentationFPS != g.draft.runtime.PresentationFPS {
					rl.SetTargetFPS(int32(g.draft.runtime.PresentationFPS))
				}
			}
			if g.row == 3 {
				windowChanged = true
			}
			if before.meter != g.draft.meter {
				*meter = presentationstats.Start(g.draft.meter)
			}
		case rl.KeyEnter, rl.KeyKpEnter:
			if alt {
				continue
			}
			previewWindow()
			if s.pending != nil && (s.placement != nil || !s.matches(*s.pending)) {
				return true, "Waiting for the window change."
			}
			if s.pending != nil && s.mode == configuration.Windowed {
				g.draft.window = s.currentWindow()
			}
			if err := g.save(*o); err != nil {
				return true, fmt.Sprintf("Graphics preferences could not be saved: %v", err)
			}
			s.pending = nil
			o.Runtime = g.draft.runtime
			o.DrawFPSCPU = g.draft.meter
			s.menu = nil
			s.consumeHeldMenuKeys()
			return true, ""
		case rl.KeyEscape:
			closing = true
		default:
			continue
		}
		g.dirty = true
	}
	if !closing {
		previewWindow()
	}
	if closing {
		if g.original.runtime != g.draft.runtime {
			applyDisplay(o.FrontEnd, g.original.runtime)
			if g.original.runtime.PresentationFPS != g.draft.runtime.PresentationFPS {
				rl.SetTargetFPS(int32(g.original.runtime.PresentationFPS))
			}
		}
		if s.currentWindow() != g.original.window || s.pending != nil {
			s.setDisplayWindow(g.original.window)
		}
		if g.original.meter != g.draft.meter {
			*meter = presentationstats.Start(g.original.meter)
		}
		s.menu = nil
		s.consumeHeldMenuKeys()
		s.pending = nil
	}
	return true, notice
}
