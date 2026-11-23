package menus

// consumeMastCyclic mirrors0975:0275..0297. The already resolved source cyclic
// words feed the sight, then both words are consumed before aircraft physics.
// Old films retain the pre-R221 keyboard-only consumer. R221 needs no new input
// payload: Live.Axes/Digital already record the actual application-tick operands.
func (p *RenderingPreview) consumeMastCyclic() {
	r := p.Flight
	if r == nil || p.ViewControls == nil || p.ViewControls.Mode != 7 {
		return
	}
	if p.fieldCorrectionsMechanics && r.commandInput.Live != nil {
		p.ViewControls.MastControls(r.Axes.X, r.Axes.Y)
	} else {
		p.ViewControls.MastKeyboard(r.held.Left, r.held.Right, r.held.Up, r.held.Down)
	}
	r.Axes.Clear()
}
