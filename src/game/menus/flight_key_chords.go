package menus

// utilityKeyChords owns a physical key until its release after a modified
// command. Releasing Alt/Shift first must not turn that same press into chaff
// or weapon cycling. Pulses enter the ordinary shared edge/input owner; device
// bindings still merge there. This physical latch is never film/checkpoint state.
type utilityKeyChords struct {
	mConsumed, spaceConsumed bool
}

func (k *utilityKeyChords) sample(focused, live, alt, ctrl, shift, m, space bool) (recall, jettison bool) {
	if !focused {
		return false, false // Preserve ownership while GLFW may clear held keys.
	}
	if !m {
		k.mConsumed = false
	}
	if !space {
		k.spaceConsumed = false
	}
	if alt && !ctrl && !shift && m && !k.mConsumed {
		k.mConsumed = true // Replay Alt+M owns the same physical M too.
		recall = live
	}
	if live && !alt && !ctrl && shift && space && !k.spaceConsumed {
		k.spaceConsumed = true
		jettison = true
	}
	return recall, jettison
}

// The original flight dispatcher recognizes scan1900h. Plain P is accepted as
// exercised in DOS; Alt+P retains the port's established compatibility shortcut.
// Other modifier chords keep their existing context owners.
func pauseChord(p, alt, ctrl, shift bool) bool {
	return p && !ctrl && !shift
}
