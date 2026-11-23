package replay

import "testing"

func TestWingmanDamageProfilePreservesHistoricalRules(t *testing.T) {
	for _, p := range []string{R22Compatibility, R22AuthoredCompatibility, R21Compatibility, R21AuthoredCompatibility, R12Compatibility, R1Compatibility, Compatibility, LegacyCompatibility} {
		f := &Film{Header: Header{Simulation: p, InputSchema: R1InputCompatibility}}
		want := p == R22Compatibility || p == R22AuthoredCompatibility
		if f.WingmanDamageMechanics() != want {
			t.Fatalf("%s wingman victim interpretation changed", p)
		}
		if want && (!f.StationaryEnemyMechanics() || !f.NavigationMechanics() || !f.SupportInput() || f.Authored() != (p == R22AuthoredCompatibility)) {
			t.Fatalf("%s profile inheritance", p)
		}
	}
}
