package menus

import (
	"gunship2000-assets/game/configuration"
	sound "gunship2000-assets/game/sounds"
	"math"
)

// Viewport is in window/mouse coordinates, not framebuffer or DOS coordinates.
// 4:3 presentation corrects the non-square pixels of the original 320x200 mode.
type Viewport struct{ X, Y, Width, Height float64 }

func officeViewport(width, height int, squarePixels bool) Viewport {
	if width <= 0 || height <= 0 {
		return Viewport{}
	}
	aspect := 4.0 / 3.0
	if squarePixels {
		aspect = 320.0 / 200.0
	}
	w, h := float64(width), float64(height)
	if w/h > aspect {
		w = math.Floor(h * aspect)
	} else {
		h = math.Floor(w / aspect)
	}
	return Viewport{math.Floor((float64(width) - w) / 2), math.Floor((float64(height) - h) / 2), w, h}
}

func (v Viewport) LogicalPoint(x, y float64) (int, int, bool) {
	if v.Width <= 0 || v.Height <= 0 || x < v.X || y < v.Y || x >= v.X+v.Width || y >= v.Y+v.Height {
		return 0, 0, false
	}
	return int((x - v.X) * 320 / v.Width), int((y - v.Y) * 200 / v.Height), true
}

// Long host pauses discard excess work instead of replaying an unbounded backlog.
// The nominal tick is a tuning policy; it is not a measured DOS refresh rate.
type tickAccumulator struct{ Rate, remainder float64 }

func (c *tickAccumulator) Advance(seconds float64, paused bool) int {
	if paused {
		c.remainder = 0
		return 0
	}
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	c.remainder = math.Min(c.remainder+seconds, 8/c.Rate)
	ticks := int(c.remainder*c.Rate + 1e-9)
	c.remainder -= float64(ticks) / c.Rate
	return ticks
}

type WindowOptions struct {
	OnStarted           func()
	InstallDirectory    string
	DrawFPSCPU          bool
	BrandSplash         bool   // Ordinary process startup only.
	AppDirectory        string // resolved player-data root for packed settings and controls
	Runtime             configuration.RuntimeOptions
	FrontEnd            *FrontEnd
	AudioPreferences    *sound.AudioPreferences
	SoundEffectsEnabled *bool
	MusicEnabled        *bool
	MenuSmoke           bool
	StartupSmoke        bool
	BriefingSmoke       bool
	PreflightSmoke      bool
	SceneSmoke          bool
	Development         bool
	Width, Height       int
	WindowMode          configuration.WindowMode
	SavesDirectory      string
	SquarePixels        bool
	DrawPointer         bool
	TickRate            float64
	SmokeFrames         int
	CaptureDirectory    string
}
