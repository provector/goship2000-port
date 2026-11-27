package menus

import (
	"fmt"
	"image"
	"path/filepath"
	"slices"

	"gunship2000-assets/game/configuration"
)

type displayWindow struct {
	width, height int
	x, y          int
	monitor       int
	mode          configuration.WindowMode
}

func (w displayWindow) String() string { return w.mode.Label(w.width, w.height) }

type displayPreferences struct {
	runtime configuration.RuntimeOptions
	window  displayWindow
	meter   bool
}
type graphicsOptions struct {
	original, draft     displayPreferences
	row                 int
	windowChosen, dirty bool
	windowCursor        string // Last requested list entry, even if the desktop refused its dimensions.
	image               *image.RGBA
}

func graphicsScene(c *FrontEnd) (*FrontEnd, *RenderingPreview) {
	if c == nil {
		return nil, nil
	}
	switch c.Screen {
	case ScreenScene:
	case ScreenReplay:
		if c.Viewer == nil || !c.Viewer.Practice || c.Viewer.Playback == nil {
			return nil, nil
		}
		c = c.Viewer.Playback.Engine
		if c == nil || c.Screen != ScreenScene {
			return nil, nil
		}
	default:
		return nil, nil
	}
	p := c.Rendering
	if p == nil || p.GPS != nil {
		return nil, nil
	}
	return c, p
}
func (g *graphicsOptions) values() []string {
	r := g.draft.runtime
	res := r.SimulationResolution
	if res == "" {
		if r.SimulationRenderScale == 2 {
			res = "640x480"
		} else {
			res = "320x200"
		}
	}
	backend, rate := "Integer", "10fps (Legacy)"
	if r.FloatingGeometry {
		backend = "Floating"
	}
	if r.PresentationMode == "smooth" {
		rate = "60fps"
	}
	fps := "False"
	if g.draft.meter {
		fps = "True"
	}
	return []string{res, backend, rate, g.draft.window.String(), fps}
}
func (g *graphicsOptions) cycle(delta int) {
	d := &g.draft
	var choices []string
	switch g.row {
	case 0:
		choices = configuration.DisplayResolutions()
	case 1:
		d.runtime.FloatingGeometry = !d.runtime.FloatingGeometry
		return
	case 2:
		mode := "smooth"
		if d.runtime.PresentationMode == mode {
			mode = "stepped"
		}
		d.runtime.SetFramerate(mode)
		return
	case 3:
		choices = configuration.WindowChoices()
	case 4:
		d.meter = !d.meter
		return
	}
	selected := g.values()[g.row]
	if g.row == 3 && g.windowCursor != "" {
		selected = g.windowCursor
	}
	i := slices.Index(choices, selected)
	if i < 0 && delta < 0 {
		i = 0
	}
	choice := choices[(i+delta+len(choices))%len(choices)]
	if g.row == 0 {
		_ = d.runtime.SetResolution(choice)
		return
	}
	g.windowCursor = choice
	switch choice {
	case configuration.BorderlessLabel:
		d.window.mode = configuration.Borderless
	case configuration.FullScreenLabel:
		d.window.mode = configuration.FullScreen
	default:
		d.window.mode = configuration.Windowed
		fmt.Sscanf(choice, "%dx%d", &d.window.width, &d.window.height)
	}
	g.windowChosen = true
}
func applyDisplay(c *FrontEnd, r configuration.RuntimeOptions) {
	engine, p := graphicsScene(c)
	if p == nil {
		return
	}
	for _, e := range []*FrontEnd{c, engine} {
		e.Runtime.SimulationResolution = r.SimulationResolution
		e.Runtime.SimulationRenderScale = r.SimulationRenderScale
		e.Runtime.FloatingGeometry = r.FloatingGeometry
		e.Runtime.PresentationMode = r.PresentationMode
		e.Runtime.PresentationFPS = r.PresentationFPS
	}
	p.Scene.RenderScale, p.Scene.Resolution, p.Scene.FloatingGeometry = r.SimulationRenderScale, r.SimulationResolution, r.FloatingGeometry
	p.presentation.mode = r.PresentationMode
	if n := p.Scene.TargetPixels(); n != len(p.Frame) {
		p.Frame = make([]byte, n)
		p.worldFrame = make([]byte, n)
	}
	p.presentation.referenceDirty = true
	p.Dirty = true
}
func (g *graphicsOptions) save(o WindowOptions) error {
	d, e := configuration.LoadAt(filepath.Join(o.AppDirectory, configuration.Filename), o.InstallDirectory)
	if e != nil {
		return e
	}
	r := g.draft.runtime
	d.Config.Runtime.SimulationRenderScale = r.SimulationRenderScale
	d.Config.Runtime.SimulationResolution = r.SimulationResolution
	d.Config.Runtime.FloatingGeometry = r.FloatingGeometry
	d.Config.Runtime.PresentationMode = r.PresentationMode
	d.Config.Runtime.PresentationFPS = r.PresentationFPS
	d.Config.DrawFPSCPU = g.draft.meter
	if g.windowChosen {
		d.Config.SetWindowMode(g.draft.window.mode)
		d.Config.WindowWidth, d.Config.WindowHeight = g.draft.window.width, g.draft.window.height
	}
	return d.Save()
}
func (g *graphicsOptions) compose(c *FrontEnd) (*image.RGBA, error) {
	if !g.dirty && g.image != nil {
		return g.image, nil
	}
	_, p := graphicsScene(c)
	indexed := slices.Clone(c.Frame(false))
	if p.Error != nil {
		return nil, p.Error
	}
	pal := c.CurrentPalette()
	img, e := indexedFrameRGBAInto(indexed, pal, g.image)
	if e != nil {
		return nil, e
	}
	c.applySkyPresentation(img, indexed)
	if e = c.Office.Assets.Cockpit.DrawGraphicsMenu(indexed, []string{"RESOLUTION", "RENDERING", "FRAMERATE", "WINDOW", "SHOW FPS"}, g.values(), g.row, p.Scene.Scale()); e != nil {
		return nil, e
	}
	l := p.Scene.Layout()
	for y := l.Y[52]; y < l.Y[145]; y++ {
		for x := l.X[45]; x < l.X[276]; x++ {
			img.Set(x, y, pal[indexed[y*l.Width+x]])
		}
	}
	g.image, g.dirty = img, false
	return img, nil
}
