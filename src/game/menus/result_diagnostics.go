package menus

import (
	"fmt"
	"gunship2000-assets/game/diagnostics"
	"strings"
)

// Freeze only the small evidence needed to describe the reached boundary.
// Report workers receive values, never a FrontEnd, SaveStore or running simulation.
func (c *FrontEnd) rememberResultEvidence() {
	p := c.Rendering
	if p == nil || p.Flight == nil || c.Mission == nil || c.Mission.Data == nil || c.filmIsolation != nil {
		return
	}
	c.resultWarning = ""
	id := ""
	if p.filmRecorder != nil {
		id = p.filmRecorder.Film.Header.ID
	}
	r := p.Flight
	diag := fmt.Sprintf("mission end; elapsed=%d fuel=%v damage=%v", r.ElapsedDA, r.Systems.Fuel, r.Systems.Damage)
	if m := p.Initialized; m != nil && m.Sortie != nil && m.Sortie.Result != nil {
		end := m.Sortie.Result
		diag += fmt.Sprintf(" reason=%s source_flags=%d player=%v", end.Reason, end.SourceStatus, end.Player)
	}
	diagnostics.SetMission(c.Mission.Data.World, id, diag, r.Steps)
	diagnostics.Info("career", "Freezing reached mission facts", "step", r.Steps)
	var warnings []string
	if r.Combat != nil {
		for actor, reason := range r.Combat.RouteWarnings() {
			if reason != "" {
				warnings = append(warnings, fmt.Sprintf("aircraft %d: %s", actor, reason))
			}
		}
	}
	if c.filmError != "" {
		warnings = append(warnings, c.filmError)
	}
	c.resultWarning = strings.Join(warnings, "; ")
}
