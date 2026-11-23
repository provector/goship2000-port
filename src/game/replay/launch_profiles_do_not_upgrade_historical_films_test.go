package replay

import (
	"strings"
	"testing"
)

func TestLaunchProfilesDoNotUpgradeHistoricalFilms(t *testing.T) {
	for _, p := range []string{Compatibility, AuthoredCompatibility, CurrentCompatibility, CurrentAuthoredCompatibility, P4Compatibility, P4AuthoredCompatibility, P5Compatibility, P5AuthoredCompatibility} {
		f := testFilm(strings.Repeat("5", 32))
		f.Header.Simulation = p
		if f.P4Mechanics() {
			f.Header.InputSchema = P4InputCompatibility
		}
		b, e := Marshal(f)
		if e != nil {
			t.Fatal(p, e)
		}
		g, e := Unmarshal(b)
		if e != nil {
			t.Fatal(p, e)
		}
		if g.Header.Simulation != p {
			t.Fatal("profile rewritten", p, g.Header.Simulation)
		}
		want := p == P5Compatibility || p == P5AuthoredCompatibility
		if g.P5Mechanics() != want {
			t.Fatal("new runtime admitted in historical profile", p)
		}
		if want {
			g.Header.InputSchema = InputCompatibility
			if g.Validate() == nil {
				t.Fatal("P5 admitted old input schema")
			}
		}
	}
	f := testFilm(strings.Repeat("5", 32))
	f.Header.Simulation = "native-non-ai-unknown"
	if f.Validate() == nil {
		t.Fatal("unknown profile admitted")
	}
}
