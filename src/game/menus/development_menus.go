package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

func PreviewScreen(c *FrontEnd, screen Screen) error {
	switch screen {
	case ScreenBriefing, ScreenPapers, ScreenSettings, ScreenOutfit, ScreenOrdnance, ScreenScene, ScreenSceneInitialization:
		if err := c.enterMission(); err != nil {
			return err
		}
		if c.Mission == nil {
			return fmt.Errorf("selected scratch state cannot prepare a mission: %s", c.Message)
		}
		if screen == ScreenSettings {
			if c.Mission.SettingsError != nil {
				return c.Mission.SettingsError
			}
			if c.Mission.ReadinessError != nil {
				return c.Mission.ReadinessError
			}
			if err := gamestate.PreflightReadiness(c.Mission.ReadinessRows); err != nil {
				return err
			}
		}
		if screen == ScreenScene || screen == ScreenSceneInitialization {
			final, err := finalPreflight(c.Mission)
			if err != nil {
				return err
			}
			c.Mission.Final = final
			if screen == ScreenSceneInitialization {
				c.Mission.Cockpit.APU = 2
				return c.beginRenderingInitialization(final)
			}
			return c.enterRenderingPreview(final)
		}
		if screen == ScreenOutfit {
			return c.openOutfit()
		}
		if screen == ScreenOrdnance {
			return c.openOrdnance(0)
		}
		if screen == ScreenSettings {
			c.Mission.Cockpit.Enter()
			c.enterPreflightGPS()
			c.Mission.SettingsHelp = SettingsHelpState{}
		}
		c.Screen = screen
	case ScreenOffice:
	case ScreenRoster:
		c.Screen = screen
		c.Highlight = 0
	case ScreenTheatre:
		c.Screen = screen
		c.Highlight = c.Store.State().SelectedPilot().Theatre.Raw()
	case ScreenMission:
		return c.openMission()
	case ScreenRecovery:
		snapshot, err := c.Store.RecoveryCandidate()
		if err != nil {
			return err
		}
		c.Recovery = snapshot
		c.Screen = screen
	case ScreenQuit:
		return c.Handle(Actions{Quit: true})
	default:
		return fmt.Errorf("unsupported preview screen %q", screen)
	}
	return nil
}
