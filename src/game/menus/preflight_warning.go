package menus

import (
	"errors"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
)

// Original DS:3c52/3c8a. Preparation31's optional template3c54 starts two
// bytes into the first string; keep the complete recovered fallback without
// regenerating caches or changing historical resource fingerprints.
const briefingOverloadWarning = "HELICOPTER LOADED BEYOND CAPACITY.  Outfit Helicopters."
const briefingTypesWarning = "Too many helicopter types in flight group - maximum is 3."

type preflightRejection struct {
	Slot int
	Text string
}

func (e *preflightRejection) Error() string {
	if e.Slot >= 0 {
		return fmt.Sprintf("Aircraft %d: %s", e.Slot+1, e.Text)
	}
	return e.Text
}

// The briefing's original guard is capacity, then the five-row type limit.
// Fuel/primary arming remain the existing ORDNANCE/whole-flight return guards.
func briefingReadinessWarning(rows [][36]byte) string {
	for _, row := range rows {
		if gamestate.PreflightRatio(gamestate.RowWord(row[:], 25), gamestate.RowWord(row[:], 23)) > 100 {
			return briefingOverloadWarning
		}
	}
	if len(rows) >= 5 {
		types := map[byte]bool{}
		for _, row := range rows {
			types[row[0]] = true
		}
		if len(types) > 3 {
			return briefingTypesWarning
		}
	}
	return ""
}

func (c *FrontEnd) drawPreflightWarning(frame []byte, text string) {
	if text == "" {
		return
	}
	assetdata.OfficeFill(frame, 0, 191, 320, 9, 0)
	if f := c.Office.Assets.Missions.Fonts[3]; f != nil {
		// GS.GS2 1a79:43a6..43aa and briefing 1a79:2058..205f: font3, ink0c.
		assetdata.DrawBottomTooltip(frame, f, text, 12)
	}
}

// Gameplay validation is distinct from internal diagnostics. Keep this owner
// narrow: an asset/I/O/initialization error must never become a game caption.
func (c *FrontEnd) showPreflightRejection(err error) bool {
	var rejection *preflightRejection
	if c.Mission == nil || !errors.As(err, &rejection) {
		return false
	}
	c.Message = rejection.Error() // Existing controller-probe compatibility.
	if c.Screen == ScreenOutfit && c.Mission.Outfit != nil {
		c.Mission.Outfit.Message = rejection.Error()
	} else {
		if c.Mission.PreflightWarning != rejection.Error() {
			c.emitAudioCue(gs4Denied)
		}
		c.Mission.PreflightWarning = rejection.Error()
	}
	return true
}
