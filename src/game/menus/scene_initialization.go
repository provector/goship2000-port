package menus

import (
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/diagnostics"
	"log/slog"
	"math"
)

// Native presentation policy, not a measured DOS loader duration. Each line
// must reach a presented frame; drawing and input remain nonblocking.
const SceneInitializationLineSeconds = 0.10

type sceneInitialization struct {
	Stage               int
	elapsed             float64
	presented, prepared bool
	soundPending        bool // Source GS2 starts the initialized engine cue after imports succeed.
	soundIssued         bool
	earlySoundIssued    bool // Source GS2:0323 precedes the loader/MFD messages.
}

func (c *FrontEnd) sceneInitializationPresented() bool {
	return c.Screen == ScreenSceneInitialization || c.Screen == ScreenQuit && c.previousScreen == ScreenSceneInitialization
}

func (c *FrontEnd) sceneConfirmationPresented() bool {
	return c.Screen == ScreenQuit && (c.previousScreen == ScreenScene || c.previousScreen == ScreenSceneInitialization || c.previousScreen == ScreenReplay)
}

func (c *FrontEnd) beginRenderingInitialization(final *FinalPreflightConfiguration) error {
	if c.Mission == nil || final == nil || final.Mission != c.Mission.Data {
		return fmt.Errorf("initialization needs the current accepted preparation")
	}
	a := c.Office.Assets.Cockpit
	if a == nil || len(a.ScenePresentation.Initialization) != 7 {
		return fmt.Errorf("scene initialization metadata missing; rebuild supported assets")
	}
	c.Mission.Final = final
	c.SceneEntry = &sceneInitialization{}
	c.Rendering = nil
	c.Screen = ScreenSceneInitialization
	c.Message = ""
	if diagnostics.Enabled(slog.LevelInfo) {
		diagnostics.Info("scene", "Start switch accepted; scene initialization begins", "status", a.ScenePresentation.Initialization[0].Text, "mission_type", c.Mission.Data.Inputs.MissionType, "world", c.Mission.Data.World)
	}
	return nil
}

// Only the native window calls this after EndDrawing. Headless tests explicitly
// acknowledge a diagnostic presentation; Frame itself never advances playback.
func (c *FrontEnd) MarkScenePresented() {
	if c.Screen == ScreenReplay && c.Viewer != nil {
		c.Viewer.Playback.Engine.MarkScenePresented()
		return
	}
	if c.Screen == ScreenScene && c.Rendering != nil && c.Rendering.Flight != nil {
		c.Rendering.Flight.terminalAwaitingAck = false
	}
	if c.Screen == ScreenSceneInitialization && c.SceneEntry != nil && c.fade == nil {
		c.SceneEntry.presented = true
	}
}

func (c *FrontEnd) AdvanceSceneInitialization(seconds float64) error {
	if c.Screen != ScreenSceneInitialization || c.SceneEntry == nil || c.fade != nil {
		return nil
	}
	e := c.SceneEntry
	if !e.presented || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return nil
	}
	e.elapsed += math.Min(seconds, .25)
	if e.elapsed+1e-10 < SceneInitializationLineSeconds {
		return nil
	}
	e.elapsed = 0
	e.presented = false
	if !e.prepared {
		// The real native scene/resources/guards are validated here, after the
		// opening line has been presented. Subsequent subsystem lines reproduce
		// source presentation only; they do not assert working avionics/physics.
		if err := c.enterRenderingPreview(c.Mission.Final); err != nil {
			c.SceneEntry = nil
			if c.Screen == ScreenSceneInitialization {
				c.Screen = ScreenSettings
				c.Mission.Final = nil
				c.Mission.Cockpit.APU = 0
				c.Mission.Cockpit.Ready = false
			}
			c.Message = "Scene preparation unavailable. See terminal."
			c.noteDiagnosticStatus(slog.LevelWarn, "Scene preparation failed", err, "accepted preparation retained; returned to settings or persistence-guarded office")
			return err
		}
		c.Screen = ScreenSceneInitialization
		c.SceneEntry = e
		e.prepared = true
		diagnostics.Info("scene", "Per-sortie simulation resources ready", "status", c.Message)
	}
	if e.Stage+1 < len(c.Office.Assets.Cockpit.ScenePresentation.Initialization) {
		e.Stage++
		diagnostics.Info("scene", "Initialization status", "status", c.Office.Assets.Cockpit.ScenePresentation.Initialization[e.Stage].Text, "stage", e.Stage)
		return nil
	}
	if !e.soundIssued {
		// Source1459 starts0bh(1) after the loader's final text and the later
		// world/cockpit/pose imports. Leave one presented handoff interval so
		// audio routing consumes this latch before SceneEntry is released.
		e.soundPending, e.soundIssued = true, true
		return nil
	}
	from := c.fadeSource()
	c.SceneEntry = nil
	c.Screen = ScreenScene
	diagnostics.Info("scene", "Scene ready; entered 3D", "status", c.Message, "screen", c.Screen)
	c.BeginSceneFade(from)
	return nil
}

func (c *FrontEnd) initializationFrame() []byte {
	if c.SceneEntry == nil || c.Mission == nil {
		return make([]byte, 64000)
	}
	// Source GS2 writes into the retained settings screen, using its own default
	// font rather than GS font2. Reconstruct a clean monitor to avoid tooltip ink.
	frame := renderCockpit(c.Office.Assets.Missions, c.Mission.Settings, c.Mission.Cockpit, -1)
	c.drawRadioLCD(frame)
	assetdata.OfficeFill(frame, 200, 8, 100, 56, 0)
	if err := cockpit.DrawInitialization(frame, c.Office.Assets.Cockpit, c.SceneEntry.Stage+1); err != nil {
		c.Message = err.Error()
		c.recordDiagnostic(slog.LevelWarn, "scene", "Initialization status drawing failed; scene transition remains governed by preparation", err)
	}
	return frame
}
