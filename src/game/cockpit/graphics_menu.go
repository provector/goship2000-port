package cockpit

import "fmt"

// DrawGraphicsMenu uses the pause panel's destination remap and variable-width HUD font.
func (a *Assets) DrawGraphicsMenu(dst []byte, labels, values []string, selected, scale int) error {
	if len(a.ScenePresentation.ConfirmationRemap) != 256 {
		return fmt.Errorf("pause palette unavailable")
	}
	return LogicalPass(dst, scale, func(p []byte) error {
		for y := 52; y <= 144; y++ {
			for x := 45; x <= 275; x++ {
				at := 320*y + x
				PutLogical(p, at, a.ScenePresentation.ConfirmationRemap[p[at]]+0xe8)
			}
		}
		for i, label := range labels {
			w, e := a.textWidth(label, 2)
			if e != nil {
				return e
			}
			color := byte(0x1b)
			if i == selected {
				color = 15
				if e = a.Text(p, ">", 54, 60+i*12, color, 0x12, Full); e != nil {
					return e
				}
			}
			if e = a.Text(p, label, 160-w, 60+i*12, color, 0x12, Full); e != nil {
				return e
			}
			if e = a.Text(p, ": <"+values[i]+">", 160, 60+i*12, color, 0x12, Full); e != nil {
				return e
			}
		}
		for i, msg := range []string{"UP/DOWN: SELECT   LEFT/RIGHT: CHANGE", "ENTER: SAVE   ESC: CANCEL"} {
			w, e := a.textWidth(msg, 2)
			if e != nil {
				return e
			}
			if e = a.Text(p, msg, 160-w/2, 125+i*10, 15, 0x12, Full); e != nil {
				return e
			}
		}
		return nil
	})
}
