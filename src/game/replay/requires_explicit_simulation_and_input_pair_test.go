package replay

import (
	"strings"
	"testing"
)

func TestRequiresExplicitSimulationAndInputPair(t *testing.T) {
	for _, sim := range []string{R1Compatibility, R1AuthoredCompatibility} {
		f := testFilm(strings.Repeat("a", 32))
		f.Header.Simulation = sim
		f.Header.InputSchema = R1InputCompatibility
		if e := f.Validate(); e != nil {
			t.Fatal(e)
		}
		if !f.R1Mechanics() || !f.P7Mechanics() || !HeldInput(f.Header.InputSchema) {
			t.Fatal("R1 must inherit accepted mechanics")
		}
		f.Header.InputSchema = Q32InputCompatibility
		if f.Validate() == nil {
			t.Fatal("new route epoch accepted old input")
		}
	}
	f := testFilm(strings.Repeat("b", 32))
	f.Header.Simulation = P7Compatibility
	f.Header.InputSchema = R1InputCompatibility
	if f.Validate() == nil {
		t.Fatal("old simulation admitted R1 orders")
	}
	f.Header.InputSchema = Q32InputCompatibility
	if e := f.Validate(); e != nil {
		t.Fatal("old film rejected", e)
	}
}
