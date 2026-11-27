package cockpit

import (
	"fmt"
	"strings"
)

// DrawPointMenu shares the pause palette remap and centers the variable-width text.
func (a *Assets) DrawPointMenu(dst []byte, lines []string, busy string, scale int) error {
	if len(a.ScenePresentation.ConfirmationRemap) != 256 {
		return fmt.Errorf("pause palette unavailable")
	}
	widths := make([]int, len(lines))
	half := 0
	for i, line := range lines {
		w, e := a.textWidth(line, 2)
		if e != nil {
			return e
		}
		widths[i] = w
		half = max(half, (w+1)/2)
	}
	top := 96 - 6*len(lines)
	return LogicalPass(dst, scale, func(p []byte) error {
		for y := top - 5; y < top+12*len(lines)+2; y++ {
			for x := max(0, 154-half); x <= min(319, 166+half); x++ {
				i := y*320 + x
				PutLogical(p, i, a.ScenePresentation.ConfirmationRemap[p[i]]+0xe8)
			}
		}
		if busy != "" {
			width, e := a.textWidth(busy, 2)
			if e != nil {
				return e
			}
			return a.Text(p, busy, 160-width/2, 90, 0x0a, 0x12, Full)
		}
		for i, line := range lines {
			x, y := 160-widths[i]/2, top+i*12
			if e := a.Text(p, line, x, y, 0x0a, 0x12, Full); e != nil {
				return e
			}
			if strings.HasPrefix(line, "Press ") {
				key := strings.Fields(line)[1]
				prefix, e := a.textWidth("Press ", 2)
				if e != nil {
					return e
				}
				if e = a.Text(p, key, x+prefix, y, 15, 0x12, Full); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
