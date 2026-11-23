package cockpit

import "fmt"

type WaypointDisplay struct {
	Present        bool
	Bearing, Range uint16
}
type NavigationInputs struct {
	Active int8
	Points [3]WaypointDisplay
}
type ObjectiveDisplay struct {
	Available bool
	Text      string
	Status    uint16
}

// RangeText is source style19: unsigned wrapped metric, one decimal kilometre
// label. The metric producer is2*max+min in SAR4 native coordinates, not physics
// speed, the approach-distance metric, or an invented Euclidean unit conversion.
func RangeText(v uint16) string   { return fmt.Sprintf("%d.%dkm", v/1000, (v%1000)/100) }
func BearingText(v uint16) string { return fmt.Sprintf("%03d`", v/183) }
func (a *Assets) drawNavigationMFDs(dst []byte, d Definition, in Inputs) error {
	for _, secondary := range []bool{false, true} {
		mode := in.PrimaryMFD
		base := 0x54
		if secondary {
			mode = in.SecondaryMFD
			base = 0x60
		}
		if mode < 1 || mode > 3 {
			continue
		}
		// Primary damage5fe20 and secondary5fe04 independently gate consumers.
		gate := byte(0x20)
		if secondary {
			gate = 4
		}
		if in.Damage[0]&gate != 0 {
			continue
		}
		p := func(off int) int { return int(d.Parameters[off/2]) }
		clip := Rect{p(base), p(base + 2), p(base+4) - 1, p(base+6) - 1}
		if clip.Left < 0 || clip.Top < 0 || clip.Right > 319 || clip.Bottom > 199 || clip.Left >= clip.Right || clip.Top >= clip.Bottom {
			return fmt.Errorf("navigation MFD bounds")
		}
		fill(dst, clip.Left, clip.Top, clip.Right-clip.Left+1, clip.Bottom-clip.Top+1, 0, clip)
		heading := "waypoints"
		if mode == 2 {
			heading = "primary"
		}
		if mode == 3 {
			heading = "secondary"
		}
		headingY := clip.Top + 4
		if mode == 1 {
			headingY += 2
		}
		if err := a.Text(dst, a.Literals[heading], clip.Left, headingY, 26, 0x212, clip); err != nil {
			return err
		}
		if mode == 1 && in.Route != nil {
			for i, wp := range in.Route.Points {
				if !wp.Present {
					continue
				}
				ink := byte(26)
				if int(in.Route.Active) == i+1 {
					ink = 31
				}
				y := clip.Top + 16 + 6*i
				if err := a.Text(dst, RangeText(wp.Range), clip.Left+2, y, ink, 0x12, clip); err != nil {
					return err
				}
				if err := a.Text(dst, BearingText(wp.Bearing), clip.Right+1-20, y, ink, 0x12, clip); err != nil {
					return err
				}
			}
		} else if mode >= 2 {
			o := in.Objectives[mode-2]
			if o.Available {
				text, style, y := o.Text, uint16(0x112), clip.Top+12
				if o.Status == 0 {
					text = a.Literals["achieved"]
					style = 0x212
					y = clip.Top + 16
				}
				if err := a.Text(dst, text, clip.Left+2, y, 26, style, clip); err != nil {
					return err
				}
			}
		}
		if err := a.remapMFD(dst, d, secondary, in.SystemFlags); err != nil {
			return err
		}
	}
	return nil
}
