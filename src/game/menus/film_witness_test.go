package menus

import (
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	"os"
	"testing"
)

// Explicit hash-bound private fixtures are supplied by stepP7_verify.py. A
// matching-source checkout without them honestly skips this coverage; the
// release verifier requires this root test to PASS and rejects that skip.
func TestHistoricalAndNewCompleteFilmWitnesses(t *testing.T) {
	roles := []struct {
		name, key string
		modern    bool
	}{{"recent-P6-Air", "GS2000_P7_RECENT_P6_FILM", false}, {"new-P7-MLRS-three-requests", "GS2000_P7_MLRS_FILM", true}, {"new-P7-M109-three-requests", "GS2000_P7_M109_FILM", true}, {"new-P7-feedback", "GS2000_P7_FEEDBACK_FILM", true}}
	any := false
	for _, r := range roles {
		if os.Getenv(r.key) != "" {
			any = true
		}
	}
	if !any {
		t.Skip("explicit P7 release film fixtures unavailable; use hash-bound stepP7 verifier")
	}
	for _, r := range roles {
		if os.Getenv(r.key) == "" {
			t.Fatalf("required P7 film fixture environment %s missing", r.key)
		}
	}
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		t.Run(r.name, func(t *testing.T) {
			b, e := os.ReadFile(os.Getenv(r.key))
			if e != nil {
				t.Fatal(e)
			}
			f, e := replay.Unmarshal(b)
			if e != nil {
				t.Fatal(e)
			}
			if f.P7Mechanics() != r.modern {
				t.Fatal("fixture interpretation differs", f.Header.Simulation)
			}
			q, e := NewFilmPlayback(a, f, configuration.Defaults())
			if e != nil {
				t.Fatal(e)
			}
			for !q.Done {
				if e = q.Step(); e != nil {
					t.Fatal(e)
				}
			}
			if q.Divergence != "" || q.Engine.Store != nil {
				t.Fatal("strict film/career isolation", q.Divergence)
			}
			t.Logf("strict complete %s simulation%s input%s operations%d steps%d score%d", r.name, f.Header.Simulation, f.Header.InputSchema, len(f.Operations), f.End.Steps, f.End.Score)
		})
	}
}
