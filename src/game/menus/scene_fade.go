package menus

import (
	"image"
	"math"
)

// Port presentation cadence, not a measured native fade duration. Both legs
// share the existing logical clock; indexed pixels/palettes remain immutable.
const DefaultSceneFadeMilliseconds = 150

type sceneFade struct {
	From           *image.RGBA
	Tick, LegTicks int
}

func (f *sceneFade) duration() int {
	if f.From == nil {
		return f.LegTicks
	}
	return 2 * f.LegTicks
}

func (c *FrontEnd) BeginSceneFade(from *image.RGBA) {
	if c.FadeMilliseconds <= 0 {
		return
	}
	n := int(math.Ceil(float64(c.FadeMilliseconds) * NominalOfficeTickRate / 1000))
	c.fade = &sceneFade{From: from, LegTicks: max(1, n)}
}

func (c *FrontEnd) fadeSource() *image.RGBA {
	if c.FadeMilliseconds <= 0 {
		return nil
	}
	rgba, _ := c.RenderRGBA(false)
	return rgba
}

func (c *FrontEnd) applySceneFade(frame *image.RGBA) *image.RGBA {
	f := c.fade
	if f == nil {
		return frame
	}
	amount, source := f.Tick, frame
	if f.From != nil {
		if f.Tick < f.LegTicks {
			source, amount = f.From, f.LegTicks-f.Tick
		} else {
			amount = f.Tick - f.LegTicks
		}
	}
	out := image.NewRGBA(source.Bounds())
	for i := 0; i < len(source.Pix); i += 4 {
		for channel := 0; channel < 3; channel++ {
			out.Pix[i+channel] = byte(int(source.Pix[i+channel]) * amount / f.LegTicks)
		}
		out.Pix[i+3] = 255
	}
	return out
}
