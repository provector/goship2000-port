package replay

import "testing"

func TestNavigationProfileInheritanceAndHistoricalExclusion(t *testing.T) {
	for _, profile := range []string{R11NavigationCompatibility, R11NavigationAuthoredCompatibility} {
		f := &Film{Header: Header{Simulation: profile, InputSchema: R1InputCompatibility}}
		if !f.NavigationMechanics() || !f.WaypointReleaseMechanics() || !f.CameraContactMechanics() || !f.R11Mechanics() || !f.R1Mechanics() {
			t.Fatal("new profile omitted inherited source mechanisms", profile)
		}
	}
	for _, profile := range []string{R11WaypointReleaseCompatibility, R11WaypointReleaseAuthoredCompatibility, R11AvoidanceCompatibility, R11AvoidanceAuthoredCompatibility, R11CameraCompatibility, R11CameraAuthoredCompatibility, R11Compatibility, R11AuthoredCompatibility, R1Compatibility, P7Compatibility, CurrentCompatibility, Compatibility} {
		f := &Film{Header: Header{Simulation: profile}}
		if f.NavigationMechanics() {
			t.Fatal("old film gained native navigation", profile)
		}
	}
}
