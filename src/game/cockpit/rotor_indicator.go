package cockpit

// drawRotorIndicator is section2 original1975:064e..06a9. The two four-pixel
// carriers are already prepared source sprites, not colors inferred from a
// familiar rotor lamp. Source sprite flags4 preserve zero and use the anchor
// directly; all supported CPIT anchors fit the320x200 destination.
func (a *Assets) drawRotorIndicator(dst []byte, d Definition, in Inputs) error {
	if in.Damage[0]&0x10 != 0 {
		return nil
	}
	name := "warning"
	if in.Rotor == 0 {
		collective := in.BarLoad
		if in.Bars != nil {
			collective = in.Bars.LoadOrdinal
		}
		if collective == 0 || in.BlinkPhase&4 == 0 {
			return nil
		}
		name = "driver-marker"
	}
	return a.Sprite(dst, name, int(d.Parameters[0x88/2]), int(d.Parameters[0x8a/2]), Full)
}
