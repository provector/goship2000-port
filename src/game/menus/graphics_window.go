//go:build !headless

package menus

import (
	"fmt"
	rl "github.com/gen2brain/raylib-go/raylib"
	"gunship2000-assets/internal/presentationstats"
	"slices"
	"time"
)

// The desktop owns the modal. No flight pause, replay operation or input command
// is submitted. The opening pause state and interpolation snapshots stay intact.
type graphicsWindow struct {
	menu     *graphicsOptions
	consumed map[int32]bool
	windowed displayWindow
	pending  *displayWindow
	deadline time.Time
}

// Quarantine only the menu buttons that remain physically held on exit.
// Unrelated flight inputs can be sampled on the first live frame.
func (s *graphicsWindow) consumeHeldMenuKeys() {
	for _, key := range [...]int32{rl.KeyEnter, rl.KeyKpEnter, rl.KeyEscape, rl.KeyLeft, rl.KeyRight, rl.KeyDown, rl.KeyUp, rl.KeyO} {
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

func readWindow(previous displayWindow) displayWindow {
	if rl.IsWindowFullscreen() {
		previous.full = true
		return previous
	}
	return displayWindow{rl.GetScreenWidth(), rl.GetScreenHeight(), false}
}
func (s *graphicsWindow) setDisplayWindow(w displayWindow) {
	// Fullscreen also has a windowed return size. Re-establish it when
	// cancelling a transaction that previewed a different windowed size.
	restoreSize := s.windowed.width != w.width || s.windowed.height != w.height
	if rl.IsWindowFullscreen() && (!w.full || restoreSize) {
		rl.ToggleFullscreen()
	}
	if !rl.IsWindowFullscreen() {
		if w.width > 0 && w.height > 0 && (rl.GetScreenWidth() != w.width || rl.GetScreenHeight() != w.height) {
			rl.SetWindowSize(w.width, w.height)
		}
		if w.full {
			s.windowed = displayWindow{width: w.width, height: w.height}
			rl.ToggleFullscreen()
		}
	}
}
func (s *graphicsWindow) input(o *WindowOptions, keys []int32, alt, closing bool, meter **presentationstats.Meter) (blocked bool, notice string) {
	w := readWindow(s.windowed)
	if !w.full {
		s.windowed = w
	}
	if s.menu == nil {
		if !alt || !slices.Contains(keys, int32(rl.KeyO)) || !rl.IsWindowFocused() || closing {
			return false, ""
		}
		if _, p := graphicsScene(o.FrontEnd); p == nil {
			return false, ""
		}
		prefs := displayPreferences{o.Runtime, w, o.DrawFPSCPU}
		s.menu = &graphicsOptions{draft: prefs, original: prefs, dirty: true}
		return true, ""
	}
	g := s.menu
	if s.pending != nil {
		if w == *s.pending {
			s.pending = nil
		} else if time.Now().After(s.deadline) {
			s.setDisplayWindow(g.original.window)
			g.draft.window = g.original.window
			g.windowChosen = false
			s.pending = nil
			g.dirty = true
			notice = "Window change unavailable. Restored the previous window."
		}
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
			if before.window != g.draft.window {
				s.setDisplayWindow(g.draft.window)
				v := g.draft.window
				s.pending = &v
				s.deadline = time.Now().Add(550 * time.Millisecond)
			}
			if before.meter != g.draft.meter {
				*meter = presentationstats.Start(g.draft.meter)
			}
		case rl.KeyEnter, rl.KeyKpEnter:
			if s.pending != nil {
				return true, "Waiting for the window change."
			}
			if err := g.save(*o); err != nil {
				return true, fmt.Sprintf("Graphics preferences could not be saved: %v", err)
			}
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
	if closing {
		if g.original.runtime != g.draft.runtime {
			applyDisplay(o.FrontEnd, g.original.runtime)
			if g.original.runtime.PresentationFPS != g.draft.runtime.PresentationFPS {
				rl.SetTargetFPS(int32(g.original.runtime.PresentationFPS))
			}
		}
		if g.original.window != g.draft.window || s.pending != nil {
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
