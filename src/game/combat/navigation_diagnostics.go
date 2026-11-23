package combat

import (
	"gunship2000-assets/game/diagnostics"
	"log/slog"
)

// This record describes repeated operational work, not navigation decisions.
// Actor, source route/control identity, outcome and cause own suppression;
// neither wall time nor a process-wide rate limiter hides another actor/failure.
type navigationDiagnostic struct {
	Known                          bool
	Key                            NavigationKey
	Outcome, Cause                 string
	Points                         int
	Direct, Budget                 bool
	First, Last, Repeats           uint64
	MinimumQueries, MaximumQueries int
}

func (s *State) noteNavigationDiagnostic(actor int, key NavigationKey, outcome string, points int, stats navigationPlanStats) {
	if actor < 1 || actor >= len(s.navigationDiagnostics) {
		return
	}
	if stats.Error != "" {
		s.routeWarning[actor] = stats.Error
	}
	if !diagnostics.Enabled(slog.LevelWarn) {
		return
	}
	r := &s.navigationDiagnostics[actor]
	if r.Known && r.Key == key && r.Outcome == outcome && r.Cause == stats.Error && r.Points == points && r.Direct == stats.Direct && r.Budget == stats.Budget {
		r.Last = s.Updates
		r.Repeats++
		r.MinimumQueries = min(r.MinimumQueries, stats.Queries)
		r.MaximumQueries = max(r.MaximumQueries, stats.Queries)
		return
	}
	s.flushNavigationDiagnostic(actor)
	*r = navigationDiagnostic{Known: true, Key: key, Outcome: outcome, Cause: stats.Error, Points: points, Direct: stats.Direct, Budget: stats.Budget, First: s.Updates, Last: s.Updates, MinimumQueries: stats.Queries, MaximumQueries: stats.Queries}
	level, message := slog.LevelWarn, "Native path unavailable; original avoidance retained"
	switch outcome {
	case "path":
		level, message = slog.LevelDebug, "Temporary path calculated"
	case "query failed":
		message = "Native path query failed; original avoidance retained"
	case "no path":
		message = "No bounded native path; original avoidance retained"
	}
	if !diagnostics.Enabled(level) {
		return
	}
	diagnostics.Log(level, "wingmen.navigation", message, "actor", actor, "combat_update", s.Updates, "kind", key.Kind, "route_index", key.Index, "goal_h0", key.Goal.H0, "goal_h1", key.Goal.H1, "points", points, "queries", stats.Queries, "direct", stats.Direct, "budget", stats.Budget, "error", stats.Error)
}

func (s *State) flushNavigationDiagnostic(actor int) {
	if actor < 1 || actor >= len(s.navigationDiagnostics) {
		return
	}
	r := &s.navigationDiagnostics[actor]
	if r.Known && r.Repeats != 0 && diagnostics.Enabled(slog.LevelDebug) {
		diagnostics.Debug("wingmen.navigation", "Repeated navigation diagnostics summarized", "actor", actor, "outcome", r.Outcome, "first_update", r.First, "last_update", r.Last, "repeats", r.Repeats, "goal_h0", r.Key.Goal.H0, "goal_h1", r.Key.Goal.H1, "queries_min", r.MinimumQueries, "queries_max", r.MaximumQueries)
	}
	r.Repeats = 0
}

// FlushDiagnostics is called at the reached sortie-end boundary. It has no
// simulation state/RNG/film effect and never starts work when logging is off.
func (s *State) FlushDiagnostics() {
	if !diagnostics.Enabled(slog.LevelDebug) {
		return
	}
	for actor := 1; actor < len(s.navigationDiagnostics); actor++ {
		s.flushNavigationDiagnostic(actor)
	}
}
