package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/internal/rasterlayout"
	"image"
	"image/color"
	"math"
)

// Playback consumes source-derived delay-before/hold-after commands. Zero-delay
// SS records remain ordered; only presentation waits become elapsed time.
type StartupPlayback struct {
	window       *assetdata.FrameWindow
	loadErr      error
	Assets       *assetdata.StartupAssets
	Stage, Frame int
	Done         bool
	pending      int
	before       bool
	remaining    float64
	black        []byte
	waitingAudio bool
	AudioBusy    func(stage int) bool
}

func NewStartupPlayback(assets *assetdata.StartupAssets) (*StartupPlayback, error) {
	if assets == nil || len(assets.Stages) == 0 {
		return nil, fmt.Errorf("startup requires prepared animation stages")
	}
	for _, stage := range assets.Stages {
		if stage == nil || len(stage.Frames) == 0 {
			return nil, fmt.Errorf("empty startup stage")
		}
	}
	p := &StartupPlayback{Assets: assets, black: make([]byte, assetdata.StartupPixels)}
	p.beginStage()
	return p, nil
}

func (p *StartupPlayback) beginStage() {
	if p.window != nil {
		p.window.Close()
		p.window = nil
	}
	p.waitingAudio = false
	p.Frame = -1
	p.pending = 0
	p.before = true
	if p.Stage >= len(p.Assets.Stages) {
		p.Done = true
		return
	}
	p.window = p.Assets.Stages[p.Stage].NewFrameWindow()
	p.remaining = p.Assets.Stages[p.Stage].Frames[0].DelayBeforeSeconds
}

func (p *StartupPlayback) Skip() {
	if !p.Done {
		p.Stage++
		p.beginStage()
	}
}

func (p *StartupPlayback) Advance(seconds float64) {
	if p.Done || seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return
	}
	if p.waitingAudio {
		if p.AudioBusy != nil && p.AudioBusy(p.Stage) {
			return
		}
		p.Stage++
		p.beginStage()
	}
	// Host debugger/window pauses do not accumulate an unbounded playback backlog.
	budget := math.Min(seconds, .25)
	for operations := 0; operations < 8192 && !p.Done; operations++ {
		if budget+1e-10 < p.remaining {
			p.remaining -= budget
			return
		}
		budget = math.Max(0, budget-p.remaining)
		stage := p.Assets.Stages[p.Stage]
		if p.before {
			p.Frame = p.pending
			p.before = false
			p.remaining = stage.Frames[p.Frame].HoldSeconds
		} else {
			p.pending++
			if p.pending == len(stage.Frames) {
				// PLAYER drains logical audio after visuals, not the WAV release tail.
				if p.AudioBusy != nil && p.AudioBusy(p.Stage) {
					p.waitingAudio = true
					return
				}
				p.Stage++
				p.beginStage()
				continue
			}
			p.before = true
			p.remaining = stage.Frames[p.pending].DelayBeforeSeconds
		}
	}
}

func (p *StartupPlayback) Current() ([]byte, []byte) {
	if p.Done {
		return p.black, make([]byte, 768)
	}
	stage := p.Assets.Stages[p.Stage]
	if p.Frame < 0 {
		return p.black, stage.Frames[0].PaletteRGB6
	}
	f := stage.Frames[p.Frame]
	pixels, err := p.window.Pixels(p.Frame)
	if err != nil {
		p.loadErr = err
		return p.black, f.PaletteRGB6
	}
	return pixels, f.PaletteRGB6
}

func (c *FrontEnd) BeginStartup(assets *assetdata.StartupAssets) error {
	if c.Debrief != nil {
		return nil
	} // A persisted pending result owns restart, before ordinary menus.
	playback, err := NewStartupPlayback(assets)
	if err != nil {
		return err
	}
	c.Startup = playback
	c.Screen = ScreenStartup
	c.Highlight = -1
	return nil
}

func (c *FrontEnd) AdvanceStartup(seconds float64) error {
	if c.Screen != ScreenStartup {
		return nil
	}
	c.Startup.Advance(seconds)
	if c.Startup.Done {
		c.releaseStartup()
		return c.RefreshOffice()
	}
	return nil
}

func (c *FrontEnd) releaseStartup() {
	if p := c.Startup; p != nil {
		if p.window != nil {
			p.window.Close()
		}
		p.window = nil
		p.black = nil
		p.Assets = nil
		p.AudioBusy = nil
	}
	c.Office.Assets.Startup = nil
}
func (c *FrontEnd) startupPresented() bool {
	return c.Startup != nil && (c.Screen == ScreenStartup || c.Screen == ScreenQuit && c.previousScreen == ScreenStartup)
}

func (c *FrontEnd) CurrentPalette() color.Palette {
	if c.Screen == ScreenMissionEditor || c.Screen == ScreenQuit && c.previousScreen == ScreenMissionEditor {
		return c.editorPalette()
	}
	if c.Screen == ScreenMissionFiles || c.Screen == ScreenQuit && c.previousScreen == ScreenMissionFiles {
		return c.Office.Assets.Palette
	}
	if c.Screen == ScreenReplay || c.Screen == ScreenQuit && c.previousScreen == ScreenReplay {
		return c.replayPalette()
	}
	if c.Screen == ScreenFilms || c.Screen == ScreenQuit && c.previousScreen == ScreenFilms {
		if a := c.Office.Assets.Replay; a != nil {
			return a.Palette
		}
	}
	if c.renderingPresented() && c.Rendering != nil {
		if c.Rendering.GPS != nil {
			pal, err := c.Rendering.Renderer.Palette(c.Rendering.Scene, &c.Rendering.Visual)
			if err == nil {
				return pal
			}
		}
		return c.Rendering.Palette
	}
	if c.startupPresented() {
		_, rgb := c.Startup.Current()
		return assetdata.StartupPalette(rgb)
	}
	screen := c.Screen
	if screen == ScreenQuit {
		screen = c.previousScreen
	}
	if screen == ScreenDebrief && c.Debrief != nil {
		return c.debriefPalette()
	}
	if isMissionScreen(screen) && c.Mission != nil {
		name := "FIELD.PAL"
		if screen == ScreenInterstitial {
			name = c.Mission.View.InterstitialName() + ".PAL"
		}
		return c.Office.Assets.Missions.Palettes[name]
	}
	return c.Office.Assets.Palette
}

// usesLocalQuitUIPalette is a native presentation policy. Startup already uses
// the Office UI colors; prebrief interstitials now share that readable modal.
// The source-style indexed frame and the active scene palette remain unchanged.
func (c *FrontEnd) usesLocalQuitUIPalette() bool {
	return c.Screen == ScreenQuit && (c.previousScreen == ScreenStartup || c.previousScreen == ScreenInterstitial)
}

func (c *FrontEnd) RenderRGBA(pointer bool) (*image.RGBA, error) {
	return c.renderRGBA(pointer, nil)
}

// The desktop uploads its image immediately. Captures and fade snapshots use
// RenderRGBA and own their pixels for as long as their callers need them.
func (c *FrontEnd) renderRGBA(pointer bool, reuse *image.RGBA) (*image.RGBA, error) {
	if c.Startup != nil && c.Startup.loadErr != nil {
		return nil, c.Startup.loadErr
	}

	var indexed []byte
	if reuse != nil && c.Screen == ScreenScene && c.Rendering != nil && c.Rendering.GPS == nil && !c.flightTextVisible() {
		p := c.Rendering
		if err := p.draw(c.Office.Assets.Cockpit); err != nil {
			return nil, err
		}
		indexed = p.Frame
	} else if c.Screen == ScreenStartup {
		// Conversion reads this page synchronously. Only the RGBA destination
		// leaves this call; Frame still returns an independent indexed capture.
		indexed, _ = c.Startup.Current()
	} else {
		indexed = c.Frame(pointer)
	}
	if c.renderingPresented() && c.Rendering != nil && c.Rendering.Error != nil {
		return nil, c.Rendering.Error
	}
	rgba, err := indexedFrameRGBAInto(indexed, c.CurrentPalette(), reuse)
	if err != nil {
		return nil, err
	}
	c.applySkyPresentation(rgba, indexed)
	if c.usesLocalQuitUIPalette() {
		// Keep startup/prebrief scenery in its own palette. The user-requested
		// native modal policy resolves only this box through the Office UI palette.
		ui := c.Office.Assets.Palette
		for y := 85; y < 105; y++ {
			for x := 90; x < 230; x++ {
				r, g, b, a := ui[indexed[y*320+x]].RGBA()
				rgba.SetRGBA(x, y, color.RGBA{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
			}
		}
	}
	if c.Screen == ScreenQuit && c.quitFadeBackground != nil {
		// Keep a partly faded presentation frozen behind the global modal. Only
		// the prompt uses the target screen's UI palette; cancelling resumes the
		// exact same logical fade tick, not a newly started transition.
		background := c.quitFadeBackground
		copyFrame := image.NewRGBA(background.Bounds())
		copy(copyFrame.Pix, background.Pix)
		// A fade leg may still present the320x200 settings frame while the
		// candidate scene is640x400. Modal composition follows the frozen
		// presented size, not the next scene texture.
		if rgba.Bounds() != background.Bounds() {
			rgba = nearestRGBA(rgba, background.Bounds())
		}
		left, top, right, bottom := 90, 85, 230, 105
		if c.sceneConfirmationPresented() {
			bounds, e := cockpit.SceneConfirmationBounds(c.Office.Assets.Cockpit, c.sceneEndConfirmation)
			if e != nil {
				return nil, e
			}
			left, top, right, bottom = bounds.Left, bounds.Top, bounds.Right+1, bounds.Bottom+1
		}
		if l := rasterlayout.FromPixels(background.Bounds().Dx() * background.Bounds().Dy()); l != nil {
			left, right, top, bottom = l.X[left], l.X[right], l.Y[top], l.Y[bottom]
		}
		for y := top; y < bottom; y++ {
			for x := left; x < right; x++ {
				copyFrame.SetRGBA(x, y, rgba.RGBAAt(x, y))
			}
		}
		return copyFrame, nil
	}
	if c.Screen != ScreenQuit {
		rgba = c.applySceneFade(rgba)
	}
	return rgba, nil
}
