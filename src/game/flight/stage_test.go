package flight

import "testing"

func TestStageOriginalE0Ordering(t *testing.T) {
	s := State{FlightStage: 4}
	for i, tc := range []struct {
		e0   bool
		want int16
	}{{true, 3}, {true, 2}, {true, 1}, {false, 1}, {true, 0}, {true, 0}} {
		AdvanceStage(&s, tc.e0)
		if s.FlightStage != tc.want {
			t.Fatalf("step%d: %d want%d", i, s.FlightStage, tc.want)
		}
	}
	s.FlightStage = 128
	AdvanceStage(&s, true)
	s.FlightStage = 4 // original rotor command occurs after02ba
	if s.FlightStage != 4 {
		t.Fatal("current command must win")
	}
}
