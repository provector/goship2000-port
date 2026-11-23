package combat

// SetFeedbackEnabled selects the P7 source message producers before flight.
// Existing queue widths, duplicate filtering, actor order and timing do not change.
// This policy flag is not mutable gameplay or historical FilmState data.
func (s *State) SetFeedbackEnabled(v bool) { s.feedbackEnabled = v }
