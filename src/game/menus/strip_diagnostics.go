package menus

import (
	"gunship2000-assets/game/diagnostics"
	"log/slog"
)

// Original GS roster caption, retained by Step15's original roster-entry
// witness and Step49/editor/roster-replacement-contract.md. Safe native E edit
// remains an input operation; its documentation does not replace source text.
const originalRosterPrompt = "SELECTOR #1 to post this unit.  SELECTOR #2 to erase this unit.  ESC to cancel."
const originalUnavailableRosterPrompt = "This unit is not available for duty.  SELECTOR #2 to erase this unit.  ESC to cancel."

// Message is a compatibility carrier for controller probes, error returns and
// existing tests, not a bottom-strip drawing owner. Diagnostics are emitted at
// the event/cause owner and cannot cover the source caption or gameplay notices.
func (c *FrontEnd) reportDiagnostic(subsystem, message string, cause error) {
	c.Message = message
	c.recordDiagnostic(slog.LevelWarn, subsystem, message, cause)
}
func (c *FrontEnd) reportStatus(subsystem, message string) {
	c.Message = message
	c.recordDiagnostic(slog.LevelInfo, subsystem, message, nil)
}
func (c *FrontEnd) recordDiagnostic(level slog.Level, subsystem, message string, cause error) {
	if message == "" || !diagnostics.Enabled(level) {
		return
	}
	if c.reportedDiagnostics == nil {
		c.reportedDiagnostics = make(map[string]string)
	}
	reason := message
	if cause != nil {
		reason += "\n" + cause.Error()
	}
	if c.reportedDiagnostics[subsystem] == reason {
		return
	}
	c.reportedDiagnostics[subsystem] = reason
	diagnostics.Log(level, subsystem, message, "screen", c.Screen, "error", cause)
}
