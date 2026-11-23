package menus

// sampleGPSCommandKey is the physical GPS make-event list shared by the native
// window and input regressions. M is a modal Mission Assets command; the normal
// flight M/chaff owner is still dispatched only outside the GPS modal.
func sampleGPSCommandKey(pressed func(int32) bool) string {
	key := ""
	for _, k := range [...]int32{'F', 'H', 'S', 'A', 'L', 'D', 'G', 'W', ' ', '1', '2', '3', 'B', 'N', 'C', '6', '7', 'M'} {
		if !pressed(k) {
			continue
		}
		if k == ' ' {
			key = "space"
		} else if k >= 'A' && k <= 'Z' {
			key = string(rune(k + ('a' - 'A')))
		} else {
			key = string(rune(k))
		}
	}
	return key
}

// Expansion Ctrl orders have their own make-event domain. A held Ctrl chord
// never leaks the unmodified flight key (e.g. Ctrl+A) into another handler.
func sampleWingmanOrderKey(pressed func(int32) bool) string {
	for _, k := range []int32{'H', 'L', 'B', 'D', 'G', 'W', 'S', 'A'} {
		if pressed(k) {
			return string(rune(k + 32))
		}
	}
	return ""
}
