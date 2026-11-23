package menus

// erasePresentationWrites observes a pure indexed overlay with two independent
// destination sentinels. It identifies writes even when the real ink equals the
// underlying colour. Destination remaps and transparent holes keep their actual
// predicates; no colour is interpreted as an object's identity.
func erasePresentationWrites(mask []byte, draw func([]byte) error) error {
	var scratch presentationScratch
	return scratch.erase(mask, draw)
}

// Per-view reusable overlay probes. They are not recorded state or attribution
// records, and are never shared with the outer cockpit probes during nesting.
type presentationScratch struct{ a, b []byte }

func (s *presentationScratch) erase(mask []byte, draw func([]byte) error) error {
	if len(mask) == 0 {
		return nil
	}
	if len(s.a) != len(mask) {
		s.a, s.b = make([]byte, len(mask)), make([]byte, len(mask))
	}
	a, b := s.a, s.b
	clear(a)
	for i := range b {
		b[i] = 255
	}
	if e := draw(a); e != nil {
		return e
	}
	if e := draw(b); e != nil {
		return e
	}
	for i := range mask {
		if a[i] != 0 || b[i] != 255 {
			mask[i] = 0
		}
	}
	return nil
}
