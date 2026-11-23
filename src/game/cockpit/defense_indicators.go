package cockpit

// Section2:05cc..063f passes196a (selectedVGA0666) x,y,10,6,old,new.
// CPIT96/97 are palette indices, not dimensions; its BP+0e/+10 compare/write
// differs from the fixed BP+0a/+0c width/height loops. No alpha fill is involved.
func drawDefenseIndicators(dst []byte, d Definition, in DefenseInputs, phase uint16) {
	if in.IRActive != 0 && (in.IRActive > 8 || phase&1 != 0) {
		exactReplace(dst, int(d.Parameters[0x78/2]), int(d.Parameters[0x7a/2]), 10, 6, d.Colours[0], d.Colours[1])
	}
	if in.RadarSignature != 0 {
		exactReplace(dst, int(d.Parameters[0x7c/2]), int(d.Parameters[0x7e/2]), 10, 6, d.Colours[0], d.Colours[1])
	}
}

// Panel labels and the common AP/AH text share ADB/ADD. These remaps occur even
// when the common HUD is damaged; source29c0 repeats only the camera0 panel arm.
func drawModeIndicators(dst []byte, d Definition, in Inputs) {
	if in.CameraMode != 0 {
		return
	}
	if in.AutoPilot != 0 {
		exactReplace(dst, int(d.Parameters[0x80/2]), int(d.Parameters[0x82/2]), 10, 7, d.Colours[2], d.Colours[3])
	}
	if in.AutoHover != 0 && (in.AutoHover == 1 || in.BlinkPhase&1 != 0) {
		exactReplace(dst, int(d.Parameters[0x84/2]), int(d.Parameters[0x86/2]), 10, 7, d.Colours[2], d.Colours[3])
	}
}

// Section2:286d..297f follows the target/waypoint pips. The selected graphics0
// profile uses foreground15/style2 here, independently of artwork palette indices.
func (a *Assets) drawCommonModeText(dst []byte, in Inputs) error {
	if in.Damage[0]&0x80 != 0 {
		return nil
	}
	if in.SelectedStation >= 0 && in.SelectedStation < len(in.Stations) {
		s := in.Stations[in.SelectedStation]
		if s.displayed() && (s.Quantity != 0 || in.BlinkPhase&3 == 0) {
			// Section2:28c1 pushes y=88 then x=8 for191a(x,y,text,style).
			if err := a.Text(dst, s.Name, 8, 88, 15, 2, Full); err != nil {
				return err
			}
		}
	}
	if in.AutoPilot != 0 {
		if err := a.Text(dst, a.Literals["autopilot"], 32, 8, 15, 2, Full); err != nil {
			return err
		}
	}
	if in.AutoHover != 0 && (in.AutoHover == 1 || in.BlinkPhase&1 != 0) {
		return a.Text(dst, a.Literals["autohover"], 240, 8, 15, 2, Full)
	}
	return nil
}
