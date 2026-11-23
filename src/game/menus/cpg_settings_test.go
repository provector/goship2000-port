package menus

import (
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestAllCPGChoicesReachNormalSortie(t *testing.T) {
	for choice, mask := range []uint16{0x18, 0x08, 0} {
		c := freshCompressionController(t, gamestate.MissionTraining, true, choice)
		p := c.Rendering
		if c.Mission.Settings.CPGControls != choice || p.Initialized.Switches&0x18 != mask {
			t.Fatal("choice lost at launch", choice, c.Mission.Settings.CPGControls, p.Initialized.Switches)
		}
	}
}
