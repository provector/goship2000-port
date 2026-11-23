package menus

import (
	"fmt"
	"gunship2000-assets/internal/rasterlayout"
)

// playerShakePresentation is the selected VGA driver's display-start latch.
// The existing source ACD word is authoritative; this private output is not.
type playerShakePresentation struct{ rows int16 }

// Original0975:2F2A..2F59 reaches this only after a nonzero ACD decrement.
// New zero or odd DA writes display-start0. On even DA a nonzero cameraAD1
// skips the write; cockpit0 writes SAR2 of the remaining word. This is a whole
// composed-image shift, not camera jitter and not aircraft motion.
func (s *playerShakePresentation) advance(remaining int16, phase uint16, view int) {
	if remaining == 0 || phase&1 != 0 {
		s.rows = 0
	} else if view == 0 {
		s.rows = remaining >> 2
	}
}

func (p *RenderingPreview) drawPlayerShake() error {
	if p.Flight == nil || p.Inspection || p.Flight.TerminalPresentation.Active {
		return nil // The separate terminal CRTC writer owns that frame.
	}
	if err := p.playerShake.draw(p.Frame, p.Scene.Scale()); err != nil {
		return err
	}
	if len(p.skyMask) == len(p.Frame) {
		return p.playerShake.draw(p.skyMask, p.Scene.Scale())
	}
	return nil
}

func (s playerShakePresentation) draw(dst []byte, scale int) error {
	if l := rasterlayout.FromPixels(len(dst)); l != nil && l.Mapped() {
		offset := int(uint16(uint16(byte(s.rows)) * 320))
		if offset == 0 {
			return nil
		}
		if offset%320 == 0 && offset < 64000 {
			l.ShiftRows(dst, offset/320)
			return nil
		}
		before := append([]byte(nil), dst...)
		for y := l.Top; y < l.Y[200]; y++ {
			ly := l.LogicalY[y]
			for x, lx := range l.LogicalX {
				addr := (ly*320 + lx + offset) & 0xffff
				v := byte(0)
				if addr < 64000 {
					sx, sy := addr%320, addr/320
					px := min(l.X[sx]+x-l.X[lx], l.X[sx+1]-1)
					py := min(l.Y[sy]+y-l.Y[ly], l.Y[sy+1]-1)
					v = before[py*l.Width+px]
				}
				dst[y*l.Width+x] = v
			}
		}
		return nil
	}
	if scale != 1 && scale != 2 || len(dst) != 64000*scale*scale {
		return fmt.Errorf("player display-start destination must be320x200 or640x400")
	}
	// VGA0CB6..0CC7 consumes only AL and forms the wrapped sixteen-bit
	// rows*320 start address. Fresh recognized VGA tails are zero (Step38).
	offset := int(uint16(uint16(byte(s.rows)) * 320))
	if offset == 0 {
		return nil
	}
	stride := 320 * scale
	if offset < 64000 && offset%320 == 0 {
		shift := offset * scale * scale
		copy(dst[:len(dst)-shift], dst[shift:])
		clear(dst[len(dst)-shift:])
		return nil
	}
	// Unusual retained source words can wrap beyond the1536-byte VGA tail.
	// Keep that address arithmetic rather than clamping or moving the aircraft.
	before := append([]byte(nil), dst...)
	for y := 0; y < 200*scale; y++ {
		for x := 0; x < stride; x++ {
			address := (y/scale*320 + x/scale + offset) & 0xffff
			colour := byte(0)
			if address < 64000 {
				colour = before[(address/320*scale+y%scale)*stride+address%320*scale+x%scale]
			}
			dst[y*stride+x] = colour
		}
	}
	return nil
}
