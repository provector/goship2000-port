package menus

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// Share ORDNANCE's source guards at the port's outer briefing boundary. A
// wingman may be unarmed, but no selected aircraft may be overloaded or dry.
func validatePreparedFlight(rows [][36]byte) error {
	if len(rows) == 0 {
		return fmt.Errorf("no prepared aircraft")
	}
	for slot := range rows {
		if message := ordnanceAcceptance(rows[slot][:], slot); message != "" {
			return &preflightRejection{Slot: slot, Text: message}
		}
	}
	if message := briefingReadinessWarning(rows); message != "" {
		return &preflightRejection{Slot: -1, Text: message}
	}
	return gamestate.PreflightReadiness(rows)
}
