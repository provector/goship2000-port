package menus

// selectRocketSalvo compiles source1:c7c/c8a/d14. One is unconditional;
// two/four require the selected compact definition's1000 flag and the original
// signed strict thresholds (>3, >8), not a guessed2*size eligibility rule.
func (p *RenderingPreview) selectRocketSalvo(size int16) {
	if p.Flight == nil {
		return
	}
	if size == 1 {
		p.Flight.RocketSalvo = 1
		return
	}
	if size != 2 && size != 4 || p.Initialized == nil || p.Initialized.Stores == nil || len(p.Initialized.Stores.Ships) == 0 {
		return
	}
	n := p.Initialized.SelectedStation
	if n < 0 || n >= 4 {
		return
	}
	s := p.Initialized.Stores.Ships[0].Stations[n]
	threshold := int16(3)
	if size == 4 {
		threshold = 8
	}
	if s.Flags&0x1000 != 0 && s.Ammunition > threshold {
		p.Flight.RocketSalvo = size
	}
}
