package replay

import "testing"

func TestNaturalCancelInputProfilesPreserveHistoricalHeaders(t *testing.T) {
	for _, simulation := range []string{Compatibility, AuthoredCompatibility} {
		for _, input := range []string{PreviousInputCompatibility, InputCompatibility} {
			f := testFilm("11111111111111111111111111111111")
			f.Header.Simulation = simulation
			f.Header.InputSchema = input
			if e := f.Validate(); e != nil {
				t.Fatal(simulation, input, e)
			}
		}
	}
	f := testFilm("11111111111111111111111111111111")
	f.Header.InputSchema = "logical-operations-v4"
	if e := f.Validate(); e == nil {
		t.Fatal("unknown input interpretation admitted")
	}
}
