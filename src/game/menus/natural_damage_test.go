package menus

import (
	"gunship2000-assets/game/replay"
	"testing"
)

// Normally saved gun mission, actual hostile acquisition/release/contact and
// damaged wingman return; finalack/restart/strict/Resume preserve full12 bytes.
func TestNaturalWingmanDamageReturnResultReplay(t *testing.T) {
	naturalWingmanEncounter(t, true, replay.R231AuthoredCompatibility)
}
