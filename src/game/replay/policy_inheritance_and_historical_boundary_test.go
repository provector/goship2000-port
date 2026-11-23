package replay

import "testing"

func TestPolicyInheritanceAndHistoricalBoundary(t *testing.T) {
	for _, profile := range []string{R6Compatibility, R6AuthoredCompatibility} {
		f := &Film{Header: Header{Simulation: profile}}
		if !f.R6Mechanics() || !f.SafeGameplayMechanics() || !f.ReadyRepairsMechanics() || !f.PlayerCopilotMechanics() || !f.AircraftFeedbackMechanics() || !f.EnemyBehaviorMechanics() || !f.WingmanDamageMechanics() || !f.R1Mechanics() || !f.FormationMechanics() || !f.P4Mechanics() || !f.P7Mechanics() {
			t.Fatal("profile lost accepted policy", profile)
		}
		if f.Authored() != (profile == R6AuthoredCompatibility) {
			t.Fatal("authored classification", profile)
		}
	}
	for _, p := range []string{R5Compatibility, R5AuthoredCompatibility, R4Compatibility, R3Compatibility} {
		f := &Film{Header: Header{Simulation: p}}
		if f.R6Mechanics() {
			t.Fatal("old film retagged", p)
		}
	}
}
