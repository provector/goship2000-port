package replay

import "testing"

func TestEnemyRulesAreFrozenAndHistoricalProfilesRemainDistinct(t *testing.T) {
	for _, profile := range []string{R23Compatibility, R23AuthoredCompatibility, R221Compatibility, R22Compatibility, R21Compatibility, R1Compatibility, LegacyCompatibility} {
		f := &Film{Header: Header{Simulation: profile, InputSchema: R1InputCompatibility}}
		want := profile == R23Compatibility || profile == R23AuthoredCompatibility
		if f.EnemyBehaviorMechanics() != want {
			t.Fatal(profile)
		}
		if want && (!f.FieldCorrectionsMechanics() || !f.WingmanDamageMechanics() || !f.StationaryEnemyMechanics() || !f.SupportInput() || f.Authored() != (profile == R23AuthoredCompatibility)) {
			t.Fatal("new interpretation omitted inherited owner", profile)
		}
	}
}
