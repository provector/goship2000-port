package menus

// Operational window chord only: no Action, recording or config write.
// Recognize each Enter press while Alt is down, then apply the window API only
// after both keys are released. GLFW fullscreen focus changes can synthesize
// releases followed by fresh Enter makes for a still-held physical key. Deferring
// the API avoids that ambiguity without a timer, platform hook or guessed repeat.
type windowShortcuts struct {
	enterConsumed bool
	down          bool
	pending       int
}

func (w *windowShortcuts) Read(focused, alt, enterDown, enterMake bool) (toggles int, suppress bool) {
	if !focused {
		w.pending = 0 // Leaving the app cancels a pending window gesture.
		w.enterConsumed = w.enterConsumed || enterDown
		w.down = enterDown
		return 0, w.enterConsumed
	}
	if alt && (enterMake || enterDown && !w.down) && !w.enterConsumed {
		w.pending++
		w.enterConsumed = true
	}
	suppress = w.enterConsumed || w.pending > 0
	if !enterDown && !enterMake {
		w.down = false
		w.enterConsumed = false
	} else {
		w.down = enterDown
	}
	if !alt && !enterDown && !enterMake {
		toggles = w.pending
		w.pending = 0
	}
	return toggles, suppress
}
