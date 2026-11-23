package menus

import (
	"gunship2000-assets/game/diagnostics"
	"log/slog"
)

// limitCompression is reached only from source-owned logical caller predicates.
// The recovered contact-order/incoming callers retain their own predicates.
// R12 also opts in at actual accepted healthy-follower construction, a named
// native engagement policy; generic messages/player discharge are not callers.
func (r *FlightRuntime) limitCompression() {
	old := r.Compression.Request
	r.Compression.Limit()
	if old != r.Compression.Request {
		r.Clock.ResetElapsed()
	}
}

// limitFlightCompression publishes a reached limiter to the new profile's HUD
// in the same logical step. Older films retain their preceding publishFlight
// display epoch as well as their original limiter callers.
func (p *RenderingPreview) limitFlightCompression() {
	p.limitFlightCompressionCause("source-limiter")
}

// Cause tags are diagnostic only; accepted AF3 and debt ownership stay shared.
func (p *RenderingPreview) limitFlightCompressionCause(cause string, actorTarget ...int) {
	old := p.Flight.Compression.Request
	p.Flight.limitCompression()
	if p.wingmanEngagementCompression {
		p.Cockpit.CompressionRequest = p.Flight.Compression.Request
	}
	p.traceCompressionTransition(cause, old, p.Flight.Compression.Request, actorTarget...)
}
func (p *RenderingPreview) traceCompressionTransition(cause string, before, after int16, actorTarget ...int) {
	if !p.threatFeedbackMechanics || before == after || !diagnostics.Enabled(slog.LevelDebug) {
		return
	}
	r := p.Flight
	attrs := []any{"step", r.Steps, "phase", r.ElapsedDA, "cause", cause, "requested_before", before, "effective_after", after, "height_control", r.State.HeightControl, "waypoint_index", r.State.WaypointIndex, "held_x", r.Axes.X, "held_y", r.Axes.Y, "switches", p.Initialized.Switches}
	if len(actorTarget) == 2 {
		attrs = append(attrs, "actor", actorTarget[0], "target", actorTarget[1])
	}
	diagnostics.Debug("compression", "Compression rate changed", attrs...)
}
