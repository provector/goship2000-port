package flight

// AdvanceStage is the mainloop02ba pre-command E0 operation. Run it once per
// logical update, before a current command can overwrite the cooldown with4.
func AdvanceStage(s *State, e0 bool) {
	if s != nil && e0 && s.FlightStage != 0 {
		s.FlightStage--
	}
}
