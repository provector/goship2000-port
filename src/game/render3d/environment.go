package render3d

import (
	"fmt"
	"gunship2000-assets/internal/rasterlayout"
)

// EnvironmentState owns presentation history, not weather generation or flight
// physics. Phase is a source-style logical counter; callers choose its cadence.
// RNG starts from an explicitly recorded native seed, never incidental DOS RAM.
type EnvironmentState struct {
	Switches       uint16
	Prefix         string
	Phase          uint16
	RNG            uint16
	PixelSeed      uint16
	PulseRemaining uint16
	PulseDraw      uint16 // Preserved for the later source compass consumer, not a heading override.
	CompassDraw    uint16
	CompassShown   int16
	PulseAmplitude byte
	PulseVisible   bool
	noise          []uint16
}

// EffectiveEnvironmentFlags follows GS launch8ca:0955..096b: visibility changes
// only bit0100. Clear visibility does not turn night lighting into daylight.
func EffectiveEnvironmentFlags(flags uint16, realisticVisibility bool) uint16 {
	if !realisticVisibility {
		flags &^= 0x100
	}
	return flags
}

func EnvironmentPalette(flags uint16) string {
	if flags&0x200 != 0 {
		return "DARK"
	}
	if flags&0x100 != 0 {
		return "GREY"
	}
	return "DAY"
}

// resident037f:2038 and the pixel path both use the same 16-bit Galois step.
func environmentRandom(state uint16) uint16 {
	if state&1 != 0 {
		return state>>1 ^ 0xb400
	}
	return state >> 1
}

func (e *EnvironmentState) draw() uint16 { e.RNG = environmentRandom(e.RNG); return e.RNG }

// Step mirrors the established noncombat PHIL/ANTC effect predicates/order.
// It advances once per logical visual tick, never inside drawing. Native visual
// cadence and seed are explicit compatibility policies, not recovered sim Hz.
func (e *EnvironmentState) Step(graphics byte) {
	if e == nil {
		return
	}
	e.step(graphics, true, e.Phase+1, true)
}

// StepFlight uses the flight loop's DA/E0 ownership. Weather/gust producers
// run only at E0; the compass and pixel carriers still update once per logical
// frame. Audio playback and drawing cannot advance this state.
func (e *EnvironmentState) StepFlight(graphics byte, e0 bool, phase uint16) {
	e.step(graphics, e0, phase, false)
}

// FinishFlightFrame is the later HUD000e writer: physics has already consumed
// the current AFF disturbance. Drawing only reads CompassShown.
func (e *EnvironmentState) FinishFlightFrame() {
	if e != nil {
		e.advanceCompass()
	}
}

func (e *EnvironmentState) advanceCompass() {
	e.CompassShown = int16(e.CompassDraw)
	if e.CompassDraw != 0 {
		delta := int16(e.CompassDraw) >> 1
		if e.CompassDraw&1 != 0 {
			delta = ^(int16(e.CompassDraw^0x54de) >> 1)
		}
		e.CompassDraw += uint16(delta)
	}
}

func (e *EnvironmentState) step(graphics byte, weather bool, phase uint16, compass bool) {
	if e == nil {
		return
	}
	// HUD000e..0045 reads the old disturbance before advancing its retained
	// word. Compile that mutation into the logical visual owner, not Draw.
	if compass {
		e.advanceCompass()
	}
	e.Phase = phase
	e.noise = e.noise[:0]
	if weather {
		switch e.Prefix {
		case "PHIL":
			if e.Switches&0x100 != 0 && e.Phase&0xff == 0 {
				e.PixelSeed = e.draw()
				if e.PixelSeed&1 != 0 {
					e.PixelSeed = 0
				}
			}
		case "ANTC":
			if e.Phase&0xff == 0 {
				// Source also draws for the compass disturbance carrier here. Keep
				// its random draw/order even though live heading effects are deferred.
				e.CompassDraw = e.draw()
				if e.CompassDraw&7 != 0 {
					e.CompassDraw = 0
				}
			}
			if e.PulseRemaining != 0 {
				if graphics&0x10 != 0 && e.Phase&1 == 0 {
					q := e.Phase >> 1
					if q&0x10 != 0 {
						e.PulseAmplitude = byte(q & 15)
					} else {
						e.PulseAmplitude = byte(^q & 15)
					}
					e.PulseVisible = true
				}
				e.PulseRemaining--
				if e.PulseRemaining == 0 {
					e.PulseVisible = false
				}
				// Source26a0 also draws on expiry (zero passes the low-five-bit test).
				if e.PulseRemaining&31 == 0 {
					e.PulseDraw = e.draw()
				}
			} else if e.Switches&0x100 != 0 && e.Phase&0x1ff == 0 && e.draw()&2 != 0 {
				e.PulseRemaining = uint16(byte(e.draw())) + uint16(byte(e.draw())) + 255
			}
		}
	}
	if e.PixelSeed != 0 {
		count := int(e.PixelSeed&0x1fff | 0x100)
		for i := 0; i < count; i++ {
			e.PixelSeed = environmentRandom(e.PixelSeed)
			e.noise = append(e.noise, e.PixelSeed)
		}
	}
}

// applyPixels is the source7402 indexed destination operation, after world
// composition and before cockpit restoration. It is not RGB fog or alpha rain.
func (e *EnvironmentState) applyPixels(dst, remap []byte, bottom int) error {
	if e == nil || len(e.noise) == 0 {
		return nil
	}
	layout := rasterlayout.FromPixels(len(dst))
	if layout == nil || len(remap) != 256 {
		return fmt.Errorf("weather destination/remap framing")
	}
	limit := uint16(bottom * 320) // Source IMUL truncates to its low word, bottom exclusive.
	for _, at := range e.noise {
		if at < limit {
			if len(dst) == 64000 {
				dst[int(at)] = remap[dst[int(at)]]
			} else {
				x, y := int(at)%320, int(at)/320
				for yy := layout.Y[y]; yy < layout.Y[y+1]; yy++ {
					for xx := layout.X[x]; xx < layout.X[x+1]; xx++ {
						i := yy*layout.Width + xx
						dst[i] = remap[dst[i]]
					}
				}
			}
		}
	}
	return nil
}

func (e *EnvironmentState) applyPalette(rgb []byte) {
	if e == nil || !e.PulseVisible || len(rgb) != 768 {
		return
	}
	// Source bounds against staging entry143 and wraps ADD AL before comparison.
	capRGB := [3]byte{rgb[429], rgb[430], rgb[431]}
	for _, span := range [][2]int{{16, 20}, {96, 111}, {128, 143}, {160, 175}} {
		for index := span[0]; index <= span[1]; index++ {
			for c := 0; c < 3; c++ {
				v := rgb[index*3+c] + e.PulseAmplitude
				if v > capRGB[c] {
					v = capRGB[c]
				}
				rgb[index*3+c] = v
			}
		}
	}
}
