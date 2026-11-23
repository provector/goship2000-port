package replay

import "testing"

func TestEngagementProfileIsNarrowAndHistorical(t *testing.T) {
	for _, profile := range []string{R12Compatibility, R12AuthoredCompatibility, R11NavigationCompatibility, R11NavigationAuthoredCompatibility, R11WaypointReleaseCompatibility, R11Compatibility, R1Compatibility, P7Compatibility, CurrentCompatibility, Compatibility} {
		f := &Film{Header: Header{Simulation: profile, InputSchema: R1InputCompatibility}}
		want := profile == R12Compatibility || profile == R12AuthoredCompatibility
		if f.WingmanEngagementCompression() != want {
			t.Fatal("historical film gained engagement policy", profile)
		}
		if want && (!f.NavigationMechanics() || !f.R1Mechanics() || !f.SupportInput() || f.Authored() != (profile == R12AuthoredCompatibility)) {
			t.Fatal("new profile lost inherited rules", profile)
		}
	}
}
