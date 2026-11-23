package state

import "testing"

func TestResultInheritsActualLeaderNotAdjacentAircraft(t *testing.T) {
	for _, leaderReturned := range []bool{true, false} {
		_, f := flightFixture(t)
		f.ResultContext.GameplayPolicy = SafeGameplayPolicy
		for i := 1; i < 5; i++ {
			a := &f.ResultContext.Actors[i]
			a.Kind, a.Control, a.Flags, a.Pose.H0 = 0x20, 3, 0, 200000
		}
		f.ResultContext.Actors[3].Control, f.ResultContext.Actors[3].Leader = 1, 2
		if leaderReturned {
			f.ResultContext.Actors[2].Flags = 0x80
		} else {
			f.ResultContext.Actors[4].Flags = 0x80
		}
		if err := FinalizeSortieFacts(&f); err != nil {
			t.Fatal(err)
		}
		if got := f.ResultContext.Actors[3].Flags&0x80 != 0; got != leaderReturned {
			t.Fatal("formed actor3 inherited actor4 instead of named leader2", leaderReturned, f.ResultContext.Actors)
		}
	}
}
