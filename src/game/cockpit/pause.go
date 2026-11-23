package cockpit

import "fmt"

// Section1:570..5e1 measures style2, remaps an inclusive rectangle through
// selectedVGA19a1, and draws style12 at(160-halfwidth,64). UI phase is supplied;
// drawing must not advance either the pause counter or the frozen flight clock.
func (a *Assets) DrawPause(dst []byte, phase uint16, scale int) error {
	text := a.Literals["scene-pause"]
	if text == "" || len(a.ScenePresentation.ConfirmationRemap) != 256 {
		return fmt.Errorf("prepared source pause presentation unavailable")
	}
	w, err := a.textWidth(text, 2)
	if err != nil {
		return err
	}
	half := w >> 1
	return LogicalPass(dst, scale, func(p []byte) error {
		for y := 62; y <= 70; y++ {
			for x := 156 - half; x <= 163+half; x++ {
				at := y*320 + x
				PutLogical(p, at, a.ScenePresentation.ConfirmationRemap[p[at]]+0xe8)
			}
		}
		return a.Text(p, text, 160-half, 64, byte(0x1a+(phase>>8)&3), 0x12, Full)
	})
}
