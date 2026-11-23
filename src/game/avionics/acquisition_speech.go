package avionics

// AcquisitionSpeechState owns source AB3, distinct from selected target AB9,
// candidate ordering and the target-preview zoom/history cache.
type AcquisitionSpeechState struct {
	PreviousTarget uint16
}

type AcquisitionSpeechInput struct {
	Reached       bool   // Actual logical HUD phase, camera0 or7; not a draw call.
	Target        uint16 // AB9, one-based entity row, zero means none.
	Symbol        int16  // B0B; all nonzero states reach the source announcement.
	Kind          byte   // The selected dynamic row's raw kind, not contact IFF.
	TargetX       int16  // Current signed HUD projection, including final mast camera.
	SpeechEnabled bool   // DE.bit80, independent of CPG mode and output gain.
}

// Advance compiles section2:2272..22f9. A temporary lost target does not clear
// AB3, and the source writes it before the sound gate. Re-enabling output or
// reacquiring that same row must therefore not replay a stale announcement.
func (s *AcquisitionSpeechState) Advance(in AcquisitionSpeechInput) (uint16, bool) {
	if !in.Reached || in.Target == 0 || in.Symbol == 0 || s.PreviousTarget == in.Target {
		return 0, false
	}
	s.PreviousTarget = in.Target
	if !in.SpeechEnabled {
		return 0, false
	}
	command := uint16(0)
	if in.Kind <= 1 {
		command = 0x18
	}
	// JGE/JLE include both boundaries in the center phrase; do not use ABS or
	// a view angle here. These are signed projected source-pixel offsets.
	switch {
	case in.TargetX < -32:
		command += 0x2d
	case in.TargetX > 32:
		command += 0x2e
	default:
		command += 0x30
	}
	return command, true
}
