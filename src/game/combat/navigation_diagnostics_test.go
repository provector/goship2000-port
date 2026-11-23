package combat

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"reflect"
	"strings"
	"testing"
)

func TestNavigationDiagnosticsKeepActorRouteAndFailureIdentity(t *testing.T) {
	var log bytes.Buffer
	r := diagnostics.Start(t.TempDir(), configuration.Logging{ShowLog: true, Level: "DEBUG", FileName: "unused.txt", FileLimitMB: 1}, diagnostics.Options{Console: &log})
	defer r.Close()
	s := &State{}
	before := s.FilmState()
	key := NavigationKey{Kind: 0x23, Goal: NavigationPoint{100, 200}}
	stats := navigationPlanStats{Queries: 40, Direct: true}
	for i := 0; i < 20; i++ {
		s.noteNavigationDiagnostic(1, key, "path", 1, stats)
	}
	if strings.Count(log.String(), "Temporary path calculated") != 1 {
		t.Fatal("same actor/route repeated full records", log.String())
	}
	s.noteNavigationDiagnostic(2, key, "path", 1, stats)
	key.Goal.H0++
	s.noteNavigationDiagnostic(1, key, "path", 1, stats)
	if strings.Count(log.String(), "Temporary path calculated") != 3 || !strings.Contains(log.String(), "repeats=19") {
		t.Fatal("second actor/new route hidden or repeat count lost", log.String())
	}
	s.noteNavigationDiagnostic(1, key, "query failed", 0, navigationPlanStats{Error: "first actual query cause"})
	s.noteNavigationDiagnostic(1, key, "query failed", 0, navigationPlanStats{Error: "different actual query cause"})
	if !strings.Contains(log.String(), "first actual query cause") || !strings.Contains(log.String(), "different actual query cause") {
		t.Fatal("distinct error hidden", log.String())
	}
	s.FlushDiagnostics()
	after := log.String()
	s.FlushDiagnostics()
	if log.String() != after || !reflect.DeepEqual(before, s.FilmState()) {
		t.Fatal("diagnostics duplicated a summary or changed authoritative film state")
	}
}

func TestNavigationWarningCanRecurAfterRecoveryWithDebugFiltered(t *testing.T) {
	var log bytes.Buffer
	r := diagnostics.Start(t.TempDir(), configuration.Logging{ShowLog: true, Level: "WARN", FileName: "unused.txt", FileLimitMB: 1}, diagnostics.Options{Console: &log})
	defer r.Close()
	s := &State{}
	key := NavigationKey{Kind: 0x22, Goal: NavigationPoint{20, 30}}
	failure := navigationPlanStats{Error: "query unavailable"}
	s.noteNavigationDiagnostic(3, key, "query failed", 0, failure)
	s.noteNavigationDiagnostic(3, key, "query failed", 0, failure)
	s.noteNavigationDiagnostic(3, key, "path", 1, navigationPlanStats{Direct: true})
	s.noteNavigationDiagnostic(3, key, "query failed", 0, failure)
	if strings.Count(log.String(), "query unavailable") != 2 || strings.Contains(log.String(), "level=DEBUG") {
		t.Fatal("WARN first cause after recovery was lost", log.String())
	}
}

func TestDisabledNavigationDiagnosticsHaveNoStateOrDestination(t *testing.T) {
	if diagnostics.Start(t.TempDir(), configuration.DefaultLogging(), diagnostics.Options{}) != nil {
		t.Fatal("disabled logging started")
	}
	s := &State{}
	s.noteNavigationDiagnostic(1, NavigationKey{}, "path", 1, navigationPlanStats{})
	s.FlushDiagnostics()
	if s.navigationDiagnostics != [5]navigationDiagnostic{} {
		t.Fatal("logging off accumulated operational diagnostics")
	}
}
