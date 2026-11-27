//go:build !headless

package menus

import (
	"image"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"gunship2000-assets/game/configuration"
)

type windowPlacement struct {
	target   displayWindow
	started  time.Time
	last     displayWindow
	stable   int
	lastMove time.Time
	resized  bool
}

func (s *graphicsWindow) observedClient() displayWindow {
	w := displayWindow{width: rl.GetScreenWidth(), height: rl.GetScreenHeight(), monitor: rl.GetCurrentMonitor()}
	if s.nativeWayland {
		return w
	}
	pos := rl.GetWindowPosition()
	w.x, w.y = int(pos.X), int(pos.Y)
	s.desktop.clientRect(&w)
	return w
}

func centeredClient(area image.Rectangle, width, height, left, top, right, bottom int) image.Point {
	return area.Min.Add(image.Pt(left+max(0, (area.Dx()-width-left-right)/2), top+max(0, (area.Dy()-height-top-bottom)/2)))
}

// A configure callback can report the requested size before the compositor
// has finished restoring the frame. Verify placement on subsequent polls.
// A new request replaces this transaction; manual moves after it are free.
func (s *graphicsWindow) settlePlacement() bool {
	p := s.placement
	now := time.Now()
	age := now.Sub(p.started)
	observed := s.observedClient()
	if s.desktop.restoringFrame() {
		if age >= time.Second {
			s.placement = nil
			return false
		}
		return true
	}
	if p.last == observed {
		p.stable++
	} else {
		p.last, p.stable = observed, 0
	}
	if age >= time.Second {
		s.windowed, s.placement = observed, nil
		return false
	}
	if p.stable == 0 {
		return true
	}
	if observed.width != p.target.width || observed.height != p.target.height {
		// The compositor may deliver its pre-fullscreen size after the
		// original resize. Reapply once after the frame has returned.
		if !p.resized {
			rl.SetWindowSize(p.target.width, p.target.height)
			p.resized = true
			return true
		}
		if age < 350*time.Millisecond {
			return true
		}
	}
	if s.nativeWayland {
		// Only the compositor can position native Wayland toplevels.
		s.windowed, s.placement = observed, nil
		return true
	}
	x, y := p.target.x, p.target.y
	if s.pending != nil && s.pending.mode == configuration.Windowed {
		m := p.target.monitor
		pos := rl.GetMonitorPosition(m)
		bounds := image.Rect(0, 0, rl.GetMonitorWidth(m), rl.GetMonitorHeight(m)).Add(image.Pt(int(pos.X), int(pos.Y)))
		area, left, top, right, bottom := s.desktop.workArea(bounds)
		pt := centeredClient(area, observed.width, observed.height, left, top, right, bottom)
		x, y = pt.X, pt.Y
	}
	if observed.x == x && observed.y == y {
		if p.stable >= 2 {
			s.windowed, s.placement = observed, nil
		}
		return true
	}
	if now.Sub(p.lastMove) >= 60*time.Millisecond {
		rl.SetWindowPosition(x, y)
		p.lastMove = now
	}
	return true
}
