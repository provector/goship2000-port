package menus

import "fmt"

var N6DevelopmentScenarioNames = []string{"o-rescue-view", "n6-protected-exhaustion", "n6-safe-unpowered", "n6-unsafe-contact", "n6-critical-fuel", "n6-compression", "n6-rescue", "n6-follower-empty", "n6-incoming"}
var n6ScenarioBase = map[string]string{
	"o-rescue-view":           "flight-return",
	"n6-protected-exhaustion": "fuel-empty", "n6-safe-unpowered": "fuel-empty", "n6-unsafe-contact": "fuel-empty", "n6-critical-fuel": "critical-fuel-hit", "n6-compression": "fuel-empty", "n6-rescue": "flight-return", "n6-follower-empty": "flight-return", "n6-incoming": "fuel-empty",
}

func (c *FrontEnd) configureN6Scenario(name string) error {
	p, meta := c.Rendering, c.DevelopmentScenario
	r := p.Flight
	meta.Name = name
	switch name {
	case "n6-protected-exhaustion":
		meta.Expected = "No Crashes flat ordinary depletion/contact: no visual countdown; real final facts normalize once"
	case "n6-safe-unpowered", "n6-unsafe-contact":
		// Supplied approach only. Source burn, force/contact and normalizer make outcomes.
		if err := c.developmentScenarioPose(meta.BaseTARGET, true, 3, 1, 128); err != nil {
			return err
		}
		r.State.HeightControl, r.State.Hover = 0, 0
		r.Systems.Fuel[0], r.Systems.Countdown = 0, 0
		if name == "n6-unsafe-contact" {
			r.State.Pose.Angles[2] = 0x2000
			r.State.Vertical = -28
		}
		meta.Expected = "Unprotected labelled approach; actual source contact determines accepted/rough terminal; no terminal/result preset"
		meta.Adjustments = append(meta.Adjustments, fmt.Sprintf("N6 unprotected landing setting1; supplied height3/fuel0/countdown0, hover/hold off, bank%04x vertical%d; real oldempty burn and contact", r.State.Pose.Angles[2], r.State.Vertical))
		meta.Instructions = "DEVELOPMENT " + name + ": click/release then Escape. Observe real empty burn and immediate short contact approach; unsafe bank/descent should show real terminal stages. Finalack alone commits actual normalized facts."
	case "n6-critical-fuel":
		meta.Expected = "Two real accepted603 hits; inspect every eligible nonfinal terminal transaction; final-zero preserves early exit"
	case "n6-compression":
		r.Systems.Fuel[0], r.Systems.Countdown = 100, 150
		meta.Adjustments = append(meta.Adjustments, "Supplied fuel100/countdown150 overrides old fuel fixture for a stable compression test; no damage/outcome")
		meta.Expected = "Brackets nominal1/2/4/6..16 advance normal source steps; modal/focus resets to1; no accuracy/physics retune"
		meta.Instructions = "DEVELOPMENT n6-compression: click/release Escape; press ] twice for4x (window title); [ restores1x. Test Alt+P, GPS/F10 and focus reset; no queued catch-up. Alt+Q abandons this uncommitted sortie."
	case "o-rescue-view", "n6-rescue", "n6-follower-empty":
		setup := name
		if name == "o-rescue-view" {
			setup = "n6-rescue"
		}
		if err := c.configureN6FollowerScenario(setup); err != nil {
			return err
		}
		if name == "o-rescue-view" {
			// Presentation acceptance only: retain the existing genuine ground-failure
			// setup, but start outside the strict height pickup gate so both entities
			// can be inspected before an ordinary manual descent/collection.
			r.State.Pose.Height, r.State.HeightTarget = 64, 64
			r.State.Hover, r.State.HeightControl, r.State.Collective = 1, 1, 16
			r.State.Power = 1024
			r.State.Thrust = statWord(r.Parameters, 32)
			meta.Adjustments = append(meta.Adjustments, "Step O supplied initial absolute height64/hover/hold/collective16 only; real ground follower failure and reserve activation remain unreached at setup; no film initialization claim")
			meta.Instructions = "DEVELOPMENT o-rescue-view: paused above the source pickup height gate. Escape; Shift+F5 follows active flight, PageUp cycles eligible entities (Pickup). Inspect separate helicopter/Pickup before manually descending and approaching low/slow. No automatic cargo, return or film is promised."
		}
	case "n6-incoming":
		if err := c.configureN6IncomingScenario(); err != nil {
			return err
		}
	}
	p.publishFlight()
	if name == "o-rescue-view" {
		if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
			return err
		}
		p.publishView()
	}
	p.resetPresentation()
	meta.Pose = r.State.Pose
	meta.LaunchSwitches = p.Initialized.Switches
	return nil
}
