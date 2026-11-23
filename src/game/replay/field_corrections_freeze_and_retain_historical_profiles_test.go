package replay

import "testing"

func TestFieldCorrectionsFreezeAndRetainHistoricalProfiles(t *testing.T) {
	for _, profile := range []string{R221Compatibility, R221AuthoredCompatibility, R22Compatibility, R22AuthoredCompatibility, R21Compatibility, R12Compatibility, R1Compatibility, Compatibility, LegacyCompatibility} {
		f := &Film{Header: Header{Simulation: profile, InputSchema: R1InputCompatibility}}
		want := profile == R221Compatibility || profile == R221AuthoredCompatibility
		if f.FieldCorrectionsMechanics() != want {
			t.Fatal("field correction interpretation", profile)
		}
		if want && (!f.WingmanDamageMechanics() || !f.NavigationMechanics() || !f.SupportInput() || f.Authored() != (profile == R221AuthoredCompatibility)) {
			t.Fatal("inherited recorded rules", profile)
		}
	}
}
