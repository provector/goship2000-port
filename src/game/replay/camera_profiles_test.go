package replay

import (
	"strings"
	"testing"
)

func TestCameraProfileInheritanceAndHistoricalPair(t *testing.T) {
	for _, profile := range []string{R11NavigationCompatibility, R11NavigationAuthoredCompatibility, R11WaypointReleaseCompatibility, R11WaypointReleaseAuthoredCompatibility, R11AvoidanceCompatibility, R11AvoidanceAuthoredCompatibility, R11Compatibility, R11AuthoredCompatibility, R11CameraCompatibility, R11CameraAuthoredCompatibility} {
		f := testFilm(strings.Repeat("c", 32))
		f.Header.Simulation, f.Header.InputSchema = profile, R1InputCompatibility
		if err := f.Validate(); err != nil {
			t.Fatal(profile, err)
		}
		want := profile == R11NavigationCompatibility || profile == R11NavigationAuthoredCompatibility || profile == R11WaypointReleaseCompatibility || profile == R11WaypointReleaseAuthoredCompatibility || profile == R11AvoidanceCompatibility || profile == R11AvoidanceAuthoredCompatibility || profile == R11CameraCompatibility || profile == R11CameraAuthoredCompatibility
		if f.WaypointReleaseMechanics() != (profile == R11NavigationCompatibility || profile == R11NavigationAuthoredCompatibility || profile == R11WaypointReleaseCompatibility || profile == R11WaypointReleaseAuthoredCompatibility) || f.RouteAvoidanceMechanics() != (profile == R11NavigationCompatibility || profile == R11NavigationAuthoredCompatibility || profile == R11WaypointReleaseCompatibility || profile == R11WaypointReleaseAuthoredCompatibility || profile == R11AvoidanceCompatibility || profile == R11AvoidanceAuthoredCompatibility) || f.CameraContactMechanics() != want || !f.R11Mechanics() || !f.R1Mechanics() || !f.P7Mechanics() || f.Authored() != strings.Contains(profile, "authored") {
			t.Fatal("camera/profile inheritance", profile)
		}
		f.Header.InputSchema = Q32InputCompatibility
		if f.Validate() == nil {
			t.Fatal("R11 camera profile admitted incompatible input", profile)
		}
	}
}
