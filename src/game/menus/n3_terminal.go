package menus

import (
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/internal/rasterlayout"
)

type terminalPixel struct {
	index  uint16
	colour byte
}

// terminalPresentation is a logical snapshot of section1:23b0, not a damage or
// survival decision. Extra draws consume its pixels without advancing the RNG.
type terminalPresentation struct {
	Active, Completed, Fade, Exterior bool
	Status                            byte
	SourceCountdown                   int16
	DisplayStartRows                  int
	PixelCount                        int
	Frames, RNGCalls                  uint64
	pixels                            []terminalPixel
}

type terminalPresentationInputs struct {
	View                          int
	GraphicsMode, GraphicsOptions byte
	AudioEnabled                  bool // Source DE!=0, separate from output mute/volume.
	Random                        func() uint16
}

// advance owns one reached original terminal iteration. Source stop/effect
// commands repeat each iteration; the retained audio driver owns replacement.
// Main2138 does not call23b0 with countdown0. Negative supplied counters would
// invoke signed-division/unsigned-loop quirks and are outside this bounded path.
func (t *terminalPresentation) advance(s *flight.State, in terminalPresentationInputs) ([]sceneAudioCommand, error) {
	if t.Completed || s == nil || s.Terminal == nil || s.Terminal.Status == 0 || s.Terminal.Countdown == 0 {
		return nil, nil
	}
	if s.Terminal.Countdown < 0 {
		return nil, fmt.Errorf("negative terminal countdown is outside the admitted source presentation domain")
	}
	countdown, status := s.Terminal.Countdown, s.Terminal.Status
	var effect uint16
	var count, first int
	if status&0x10 != 0 { // Bit10 takes precedence over bit04 in23c6.
		effect = 0x28
		count = 32767 / int(countdown)
	} else if status&4 != 0 {
		effect = 0x11
		first = 1
		count = 8192/int(countdown) - 1
		if count < 0 {
			count = 0
		}
	}
	if in.View == 0 && count != 0 && in.Random == nil {
		return nil, fmt.Errorf("terminal pixel producer requires the shared gameplay RNG")
	}
	t.Active, t.Status, t.SourceCountdown = true, status, countdown
	t.pixels = t.pixels[:0]
	t.DisplayStartRows = 0
	t.Exterior = in.View != 0 && effect != 0
	if effect != 0 && in.View == 0 {
		if cap(t.pixels) < count {
			// A terminal sequence has at most32767 point calls per iteration.
			// Allocate once here, never per pixel or presentation frame.
			t.pixels = make([]terminalPixel, 0, 32767)
		}
		for i := 0; i < count; i++ {
			r := in.Random()
			t.RNGCalls++
			colour := byte((i + first) & 0x0c)
			if in.GraphicsMode == 0 {
				colour = byte((i+first)&1) + 0x42
			}
			// DIV328 supplies x and y, but VGA0714 writes x+320*y without
			// clipping x320..327. Keep the resulting logical linear address.
			at := uint16(r%328) + uint16(r/328)*320
			t.pixels = append(t.pixels, terminalPixel{at, colour})
		}
		if countdown&1 == 0 {
			t.DisplayStartRows = 2
			if status&0x10 != 0 {
				t.DisplayStartRows = 4
			}
		}
	}
	t.PixelCount = len(t.pixels)
	t.Frames++
	t.Completed = flight.AdvanceTerminal(s)
	// Source2530 DEC->0 longjmps through CRT0b99. Noise and audio above
	// precede that barrier; the palette/DE80/AF3 epilogue is not reached.
	t.Fade = !t.Completed && in.GraphicsOptions&0x10 != 0 && in.View != 11 && in.GraphicsMode == 0
	if !in.AudioEnabled || effect == 0 {
		return nil, nil
	}
	return []sceneAudioCommand{{event: 0x0b}, {event: effect}}, nil
}

// draw consumes the immutable latest logical pixel transaction after cockpit
// composition. Scale2 enlarges these source pixels, not their logical addresses.
// Each caller starts from its complete current image, never the previous output.
func (t *terminalPresentation) draw(dst []byte, scale int) error {
	return t.drawCovered(dst, nil, scale)
}
func (t *terminalPresentation) drawCovered(dst, owners []byte, scale int) error {
	l := rasterlayout.FromPixels(len(dst))
	if l == nil {
		return fmt.Errorf("unsupported terminal destination")
	}
	if !t.Active {
		return nil
	}
	for _, p := range t.pixels {
		x, y := int(p.index)%320, int(p.index)/320
		for py := l.Y[y]; py < l.Y[y+1]; py++ {
			for px := l.X[x]; px < l.X[x+1]; px++ {
				at := py*l.Width + px
				dst[at] = p.colour
				if len(owners) > 0 {
					owners[at] = 0
				}
			}
		}
	}
	if t.DisplayStartRows != 0 {
		if t.DisplayStartRows < 0 || t.DisplayStartRows >= 200 {
			return fmt.Errorf("invalid terminal display-start row")
		}
		l.ShiftRows(dst, t.DisplayStartRows)
		if len(owners) > 0 {
			l.ShiftRows(owners, t.DisplayStartRows)
		}
	}
	return nil
}

// fadeTerminalRGB6 is source2582..25a4 on a caller-owned selected palette copy.
// Invoke once per logical terminal iteration under the plan's Fade predicate.
func fadeTerminalRGB6(rgb []byte) error {
	if len(rgb) != 768 {
		return fmt.Errorf("terminal fade requires one private768byte RGB6 palette")
	}
	for i, v := range rgb {
		rgb[i] = v - (v >> 2)
	}
	return nil
}
