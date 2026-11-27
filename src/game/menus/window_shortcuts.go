package menus

import "gunship2000-assets/game/configuration"

// GLFW queues genuine makes separately from keyboard repeat. Wait for F11 to
// come up before changing the desktop, which can synthesize focus/key events.
// Each make retains its original modifier even if Alt is released first.
type windowShortcuts struct {
	pending             []configuration.WindowMode
	blockedUntilRelease bool
}

func (s *windowShortcuts) read(allowed, alt, otherModifier, down bool, makes int, current configuration.WindowMode) (configuration.WindowMode, bool) {
	if !allowed {
		s.pending = s.pending[:0]
		s.blockedUntilRelease = down
		return current, false
	}
	if s.blockedUntilRelease {
		s.blockedUntilRelease = down
		return current, false
	}
	if !otherModifier {
		style := configuration.Borderless
		if alt {
			style = configuration.FullScreen
		}
		for range makes {
			s.pending = append(s.pending, style)
		}
	}
	if down || len(s.pending) == 0 {
		return current, false
	}
	target := current
	for _, style := range s.pending {
		if target == style {
			target = configuration.Windowed
		} else {
			target = style
		}
	}
	s.pending = s.pending[:0]
	return target, target != current
}
