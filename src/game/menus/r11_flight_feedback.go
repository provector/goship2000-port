package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/cockpit"
)

// Source GS2 resident0:c2a receives actual tracked OBJECT row and center160/160.
// Draw only into the final presentation copy: original film/canonical frame
// feedback and simulation sensing never consume these labels.
func (c *FrontEnd) drawFlightMemberCaption(dst []byte) {
	if c.Rendering == nil {
		return
	}
	c.drawFlightMemberCaptionAtScale(dst, c.Rendering.Scene.Scale())
}
func (c *FrontEnd) drawFlightMemberCaptionAtScale(dst []byte, scale int) {
	p := c.Rendering
	if p == nil || !p.r11Mechanics || p.ViewControls == nil || p.ViewControls.Mode != 4 || p.Initialized == nil || c.Office == nil || c.Office.Assets.Cockpit == nil {
		return
	}
	a := c.Office.Assets.Cockpit
	for _, e := range p.Initialized.Entities {
		if e.ID != p.ViewControls.TrackedID {
			continue
		}
		name := stringTrimZero(e.Object.Raw[15:27])
		drawSourceMemberLine(a, dst, name, 160, scale)
		slot := e.FlightSlot
		if slot <= 0 || slot >= 5 || e.Object.Raw[10]&0x20 == 0 || p.Flight == nil {
			return
		}
		r := p.Flight.Navigation[slot].Raw
		mode := int(int8(r[1]))
		caption, detail := sourceMemberCaption(mode, int16(e.Pose.Height), r)
		if caption != "" {
			drawSourceMemberLine(a, dst, caption, 167, scale)
		}
		if detail != "" {
			drawSourceMemberLine(a, dst, detail, 174, scale)
		}
		return
	}
}

// Source c36 sets packed ink151f: foreground1f, shadow15. Text style832
// is centered with the one-pixel diagonal shadow; draw both source colors.
func drawSourceMemberLine(a *cockpit.Assets, dst []byte, s string, y, scale int) {
	_ = a.TextScaled(dst, s, 161, y+1, 0x15, 0x802, cockpit.Full, scale)
	_ = a.TextScaled(dst, s, 160, y, 0x1f, 0x802, cockpit.Full, scale)
}

// Literal table source2a29:01d7; signed height <=2 has the separate LANDED path.
func sourceMemberCaption(mode int, height int16, r [70]byte) (string, string) {
	if mode == 0 {
		return "[ OUT OF ACTION ]", ""
	}
	if mode == 2 && height <= 2 {
		return "[ LANDED ]", ""
	}
	names := [9]string{"", "[ FORMED UP ]", "[ HOLDING ]", "[ FLY TO ]", "[ RE-GROUPING ]", "[ DIS-ENGAGING ]", "[ LANDING ]", "[ LANDING ]", "[ TO BASE ]"}
	if mode < 1 || mode > 8 {
		return "", ""
	}
	alt := int(int16(binary.LittleEndian.Uint16(r[6:])))
	speed := int(int16(binary.LittleEndian.Uint16(r[8:])))
	if alt <= 0 || alt > 4 || mode == 6 || mode == 1 {
		return names[mode], ""
	}
	detail := "[ alt.: " + [5]string{"", "NOE", "CONTOUR", "LOW", "LOW"}[alt]
	if speed > 0 && speed <= 3 && mode != 2 {
		detail += " - speed: " + [4]string{"", "SLOW ]", "MEDIUM ]", "HIGH ]"}[speed]
	} else {
		detail += " ]"
	}
	return names[mode], detail
}

func (c *FrontEnd) drawLockedTargetDossier(dst []byte) bool {
	if c.Rendering == nil {
		return false
	}
	return c.drawLockedTargetDossierAtScale(dst, c.Rendering.Scene.Scale())
}
func (c *FrontEnd) drawLockedTargetDossierAtScale(dst []byte, scale int) bool {
	p := c.Rendering
	if p == nil || !p.r11Mechanics || p.Initialized == nil || p.Flight == nil || p.Flight.Avionics == nil || p.Flight.Avionics.Target == 0 || c.Office == nil || c.Office.Assets.Cockpit == nil {
		return false
	}
	row := int(p.Flight.Avionics.Target) - 1
	if row < 0 || row >= len(p.Initialized.Entities) {
		return false
	}
	a := c.Office.Assets.Cockpit
	model := int(p.Initialized.Entities[row].Object.Raw[12])
	d, known := a.TargetDossiers[model]
	if !known || d[0] == "" {
		return false
	}
	if err := cockpit.DrawTargetDossierScaled(dst, a, scale, p.Scene.View.Center.X, d[0], d[1], d[2], d[3]); err != nil {
		return false
	}
	return true
}
