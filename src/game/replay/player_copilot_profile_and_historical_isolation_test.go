package replay

import "testing"

func TestPlayerCopilotProfileAndHistoricalIsolation(t *testing.T) {
	for _, p := range []string{R3Compatibility, R3AuthoredCompatibility, R231Compatibility, R231AuthoredCompatibility, R23Compatibility, R22Compatibility, LegacyCompatibility} {
		f := &Film{Header: Header{Simulation: p, InputSchema: R1InputCompatibility}}
		want := p == R3Compatibility || p == R3AuthoredCompatibility
		if f.PlayerCopilotMechanics() != want {
			t.Fatal("unexpected positive58F2 interpretation", p)
		}
		if want && (!f.AircraftFeedbackMechanics() || !f.EnemyBehaviorMechanics() || !f.WingmanDamageMechanics() || !f.SupportInput() || f.Authored() != (p == R3AuthoredCompatibility)) {
			t.Fatal("missing inherited simulation contract", p)
		}
	}
}
