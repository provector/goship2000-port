package replay

import "testing"

func TestProfilesKeepHistoricalInterpretations(t *testing.T) {
	for _, sim := range []string{Compatibility, AuthoredCompatibility, CurrentCompatibility, CurrentAuthoredCompatibility} {
		for _, input := range []string{InputCompatibility, PreviousInputCompatibility} {
			f := testFilm("11111111111111111111111111111111")
			f.Header.Simulation = sim
			f.Header.InputSchema = input
			if e := f.Validate(); e != nil {
				t.Fatal(sim, input, e)
			}
		}
	}
	for _, sim := range []string{P4Compatibility, P4AuthoredCompatibility} {
		f := testFilm("11111111111111111111111111111111")
		f.Header.Simulation = sim
		f.Header.InputSchema = P4InputCompatibility
		if e := f.Validate(); e != nil || !f.P4Mechanics() || !f.FormationMechanics() {
			t.Fatal(sim, e)
		}
		f.Header.InputSchema = InputCompatibility
		if e := f.Validate(); e == nil {
			t.Fatal("new simulation accepted old input interpretation")
		}
	}
}
