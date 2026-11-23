package menus

import "gunship2000-assets/game/configuration"

// applyReplayPresentationOptions overlays only the three existing presentation
// settings. The recorded GPS/input/control settings remain part of the film's
// simulation profile; viewing cannot replace those gameplay inputs.
func applyReplayPresentationOptions(p *RenderingPreview, engine *FrontEnd, options configuration.RuntimeOptions) {
	opts := options.Effective()
	p.Scene.RenderScale = opts.SimulationRenderScale
	p.Scene.FloatingGeometry = opts.FloatingGeometry
	p.Scene.Resolution = opts.SimulationResolution
	if n := p.Scene.TargetPixels(); len(p.Frame) != n {
		p.Frame = make([]byte, n)
		p.worldFrame = make([]byte, n)
	}
	p.presentation.mode = opts.PresentationMode
	engine.Runtime.FloatingGeometry = opts.FloatingGeometry
	engine.Runtime.SimulationResolution = opts.SimulationResolution
	engine.Runtime.SimulationRenderScale = opts.SimulationRenderScale
	engine.Runtime.PresentationMode = opts.PresentationMode
	engine.Runtime.PresentationFPS = opts.PresentationFPS
	p.resetPresentation()
}
