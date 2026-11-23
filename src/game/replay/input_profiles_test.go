package replay

import (
	"strings"
	"testing"
)

func TestInputProfileRetainsSimulationAndHistoricalReaders(t *testing.T) {
	for _, simulation := range []string{P4Compatibility, P4AuthoredCompatibility, P5Compatibility, P5AuthoredCompatibility} {
		for _, input := range []string{P4InputCompatibility, P6InputCompatibility} {
			f := testFilm(strings.Repeat("6", 32))
			f.Header.Simulation = simulation
			f.Header.InputSchema = input
			data, e := Marshal(f)
			if e != nil {
				t.Fatal(simulation, input, e)
			}
			g, e := Unmarshal(data)
			if e != nil || g.Header.Simulation != simulation || g.Header.InputSchema != input {
				t.Fatal("explicit interpretation rewritten", simulation, input, e)
			}
		}
	}
	for _, simulation := range []string{LegacyCompatibility, Compatibility, AuthoredCompatibility, CurrentCompatibility, CurrentAuthoredCompatibility} {
		f := testFilm(strings.Repeat("6", 32))
		f.Header.Simulation = simulation
		f.Header.InputSchema = P6InputCompatibility
		if f.Validate() == nil {
			t.Fatal("v5 input silently extended pre-Support simulation", simulation)
		}
	}
}

func TestTrainInterpretationSelectedSeparatelyFromInput(t *testing.T) {
	for _, profile := range []string{P6Compatibility, P6AuthoredCompatibility} {
		f := testFilm(strings.Repeat("6", 32))
		f.Header.Simulation, f.Header.InputSchema = profile, P6InputCompatibility
		if !f.P6Mechanics() || !f.P5Mechanics() || !f.FormationMechanics() || f.Authored() != (profile == P6AuthoredCompatibility) {
			t.Fatal("new source Train interpretation", profile)
		}
		b, e := Marshal(f)
		if e != nil {
			t.Fatal(e)
		}
		g, e := Unmarshal(b)
		if e != nil || g.Header.Simulation != profile {
			t.Fatal("explicit Train profile", e)
		}
		f.Header.InputSchema = P4InputCompatibility
		if f.Validate() == nil {
			t.Fatal("new Train profile admitted old overlapping GPS input")
		}
	}
	for _, profile := range []string{P5Compatibility, P5AuthoredCompatibility} {
		f := testFilm(strings.Repeat("5", 32))
		f.Header.Simulation = profile
		if f.P6Mechanics() {
			t.Fatal("old mission rows silently acquired kind08 visits")
		}
	}
}
