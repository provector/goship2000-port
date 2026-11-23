package combat

import (
	"reflect"
	"testing"
)

func TestGuidanceDiagnosticOnlyOnActualStateChange(t *testing.T) {
	run := func(enabled bool) (*State, int) {
		s, in, _ := attackFixture(t)
		s.SetPersistentPlayerThreat(enabled)
		p := incomingAtTarget(s, 0x43, 0)
		p.Pose.H1 -= 16384 // outside physical AEB's512-unit range in both readers
		p.Speed = 1
		p.Parameter = 64
		in.Incoming.Defenses.ChaffTimer = 4
		in.Incoming.Defenses.Decoy.Height = 1024
		draws := 0
		in.Random = func() uint16 { draws++; return 1 }
		if err := s.SupplyIncoming(20, p, false); err != nil {
			t.Fatal(err)
		}
		if err := s.Step(in); err != nil {
			t.Fatal(err)
		} // establishes real shared35E0
		s.init.Entities[1].Participation = 0
		for visit := 0; visit < 3; visit++ {
			if err := s.Step(in); err != nil {
				t.Fatal(err)
			}
			if s.Slots[20].Flags != 0x23 || s.Slots[20].Parameter != 0 || s.Slots[20].Lifetime == 0 {
				t.Fatal("actual live disabled state", s.Slots[20])
			}
		}
		want := 0
		if enabled {
			want = 1
		}
		if got := len(events(s, "hostile-guidance-defeated")); got != want {
			t.Fatal("repeated consumer is not a new guidance transition", got, want)
		}
		return s, draws
	}
	current, draws := run(true)
	historical, oldDraws := run(false)
	if draws != oldDraws || !reflect.DeepEqual(current.FilmState(), historical.FilmState()) {
		t.Fatal("diagnostic changed authoritative state or RNG", draws, oldDraws)
	}
}
