package flight

// AdvanceTerminal is the2138→23b0→2530 countdown, once per reached logical
// main iteration, not E0 and never per draw. The contact frame consumes the first
// count. Native crash particles/audio/result successors remain separately owned.
func AdvanceTerminal(s *State) bool {
	if s == nil || s.Terminal == nil || s.Terminal.Status == 0 {
		return false
	}
	if s.Terminal.Countdown != 0 {
		s.Terminal.Countdown--
	}
	return s.Terminal.Countdown == 0
}
