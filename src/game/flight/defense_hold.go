package flight

// SetManualHeld is the Q3.1 input policy. The old Activate toggles remain intact
// for historical operations. A radar token is drawn only on off->held; release
// clears it. IR retains/depletes its original charge rather than refilling it.
func (d *Defenses) SetManualHeld(held [2]bool, damage byte, random func() uint16) {
	if damage&0x40 != 0 {
		held = [2]bool{}
	}
	if !held[0] {
		d.RadarSignature = 0
	} else if d.RadarSignature == 0 && random != nil {
		d.RadarSignature = (random() & 7) + 1
	}
	if !held[1] {
		if d.IRActive != 0 {
			d.IRCharge = d.IRActive
		}
		d.IRActive = 0
	} else if d.IRActive == 0 {
		d.IRActive = d.IRCharge
	}
}
