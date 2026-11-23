package menus

import (
	"gunship2000-assets/game/diagnostics"
	"log/slog"
)

// Observational only, never part of checkpoints/operations or player text ownership.
func (c *FrontEnd) noteDiagnosticStatus(level slog.Level, event string, err error, outcome string) {
	if !diagnostics.Enabled(level) {
		return
	}
	c.diagnosticStatus = c.Message
	diagnostics.Log(level, "scene", event, "status", c.Message, "screen", c.Screen, "outcome", outcome, "error", err)
}

func (c *FrontEnd) noteRecordingStatus(event string, err error, outcome string) {
	if err != nil && c.filmError == "" {
		c.filmError = event + ": " + err.Error()
	}
	if !diagnostics.Enabled(slog.LevelWarn) {
		return
	}
	c.diagnosticStatus = c.Message
	diagnostics.Warn("replay", event, "status", c.Message, "screen", c.Screen, "outcome", outcome, "error", err)
}
