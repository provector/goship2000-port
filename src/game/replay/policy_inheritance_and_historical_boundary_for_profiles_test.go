package replay

import "testing"

func TestPolicyInheritanceAndHistoricalBoundaryForProfiles(t *testing.T) {
	for _, profile := range []string{R8Compatibility, R8AuthoredCompatibility} {
		f := &Film{Header: Header{Simulation: profile}}
		if !f.R8Mechanics() || !f.R6Mechanics() || !f.SafeGameplayMechanics() || !f.ReadyRepairsMechanics() || !f.PlayerCopilotMechanics() || !f.AircraftFeedbackMechanics() || !f.EnemyBehaviorMechanics() || !f.WingmanDamageMechanics() || !f.NavigationMechanics() || !f.R1Mechanics() || !f.P7Mechanics() || !f.P4Mechanics() || !f.FormationMechanics() {
			t.Fatal(profile, "lost inherited policy")
		}
		if f.Authored() != (profile == R8AuthoredCompatibility) {
			t.Fatal(profile, "wrong authored classification")
		}
	}
	for _, profile := range []string{R6Compatibility, R6AuthoredCompatibility, R5Compatibility, R5AuthoredCompatibility, R3Compatibility} {
		if (&Film{Header: Header{Simulation: profile}}).R8Mechanics() {
			t.Fatal("old film retagged", profile)
		}
	}
}
