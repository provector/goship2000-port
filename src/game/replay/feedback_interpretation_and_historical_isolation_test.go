package replay

import "testing"

func TestFeedbackInterpretationAndHistoricalIsolation(t *testing.T) {
	for _, p := range []string{R231Compatibility, R231AuthoredCompatibility, R23Compatibility, R23AuthoredCompatibility, R221Compatibility, R22Compatibility, LegacyCompatibility} {
		f := &Film{Header: Header{Simulation: p, InputSchema: R1InputCompatibility}}
		new := p == R231Compatibility || p == R231AuthoredCompatibility
		if f.AircraftFeedbackMechanics() != new {
			t.Fatal(p)
		}
		if new && (!f.EnemyBehaviorMechanics() || !f.FieldCorrectionsMechanics() || !f.WingmanDamageMechanics() || !f.SupportInput() || f.Authored() != (p == R231AuthoredCompatibility)) {
			t.Fatal("inheritance", p)
		}
	}
}
