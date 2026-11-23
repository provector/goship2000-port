package combat

// SetPersistentPlayerThreat selects the R3 accumulator interpretation before
// any tick. Historical readers keep the per-pass reset; new58F2 is the only
// reached consumer allowed to clear AEB after an eligible defense visit.
func (s *State) SetPersistentPlayerThreat(enabled bool) { s.persistentPlayerThreat = enabled }
