package replay

import "testing"

func TestHostileProfileDoesNotChangeHistoricalMeaning(t *testing.T) {
	for _, p := range []string{R21Compatibility, R21AuthoredCompatibility, R12Compatibility, R1Compatibility, P7Compatibility, Compatibility} {
		f := &Film{Header: Header{Simulation: p, InputSchema: R1InputCompatibility}}
		want := p == R21Compatibility || p == R21AuthoredCompatibility
		if f.StationaryEnemyMechanics() != want {
			t.Fatal(p)
		}
		if want && (!f.SupportInput() || !f.WingmanEngagementCompression() || !f.NavigationMechanics() || f.Authored() != (p == R21AuthoredCompatibility)) {
			t.Fatal("profile inheritance", p)
		}
	}
}
