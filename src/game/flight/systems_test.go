package flight

import "testing"

func TestSourceFuelAndEngineCountdown(t *testing.T) {
	s := NewSystems([]int16{2, 3}, []bool{true, false})
	for i := 0; i < 150; i++ {
		s.Advance(true, 16)
		s.EaseEngines(true, true, true)
	}
	if s.Fuel[0] != 2 || s.Countdown != 0 || s.Engines != [2]int16{15, 15} {
		t.Fatalf("before original zero test%+v", s)
	}
	s.Advance(false, 16)
	if s.Countdown != 0 {
		t.Fatal("ungated update consumed countdown")
	}
	s.Advance(true, 16)
	if s.Fuel[0] != 1 || s.Fuel[1] != 3 || s.Countdown != 134 {
		t.Fatalf("reset/participation%+v", s)
	}
	for i := 0; i < 135; i++ {
		s.Advance(true, 0)
	}
	if s.Fuel[0] != 0 || s.Damage[9] != 0 {
		t.Fatalf("positive oldfuel branch must not set609%+v", s)
	}
	for i := 0; i < 151; i++ {
		s.Advance(true, 0)
	}
	if s.Fuel[0] != 0 || s.Damage[9] != 1 {
		t.Fatalf("next oldempty burn source flag%+v", s)
	}
}

func TestFuelOldValueAndPartial603Boundaries(t *testing.T) {
	// Original section1:04bf selects the old signed word, 039d masks603&3,
	// and03af prevents the second DEC at zero. These hand cases distinguish
	// the two original branches instead of deriving expectations from Advance.
	for _, tc := range []struct {
		old, want int16
		damage603 byte
		want609   byte
	}{
		{-1, -1, 0, 1}, {0, 0, 0, 1}, {1, 0, 0, 0}, {2, 1, 0, 0},
		{1, 0, 1, 0}, {2, 0, 1, 0}, {3, 1, 1, 0},
		{3, 1, 5, 0}, {3, 2, 2, 0}, {3, 2, 3, 0},
		{10, 8, 1, 0}, {24, 22, 1, 0},
	} {
		s := NewSystems([]int16{tc.old}, []bool{true})
		s.Countdown, s.Damage[3] = 0, tc.damage603
		got := s.Advance(true, 16)
		if s.Fuel[0] != tc.want || s.Damage[9] != tc.want609 || s.Countdown != 134 || got != (FuelBurn{true, tc.old}) {
			t.Fatalf("old%d damage603=%d: fuel%d/609%d/burn%+v", tc.old, tc.damage603, s.Fuel[0], s.Damage[9], got)
		}
	}
}

func TestFuelLogicalGatesAndActorOwnership(t *testing.T) {
	s := NewSystems([]int16{3, 3, 3, 3, 3, 3}, []bool{true, true, false, true, true, true})
	s.Countdown, s.Damage[3] = 0, 1
	before := s
	if got := s.Advance(false, 16); got != (FuelBurn{}) || s.Countdown != before.Countdown || s.Fuel[0] != 3 || s.Damage[9] != 0 {
		t.Fatal("E0false mutated countdown/fuel or published a burn")
	}
	got := s.Advance(true, 16)
	wantFuel := []int16{1, 2, 3, 2, 2, 3}
	for i, want := range wantFuel {
		if s.Fuel[i] != want {
			t.Fatalf("actor%d got%d want%d; player603 must not alias followers/reserve", i, s.Fuel[i], want)
		}
	}
	if got != (FuelBurn{true, 3}) {
		t.Fatalf("player event%+v", got)
	}
	s.Active[0], s.Countdown = false, 0
	if got := s.Advance(true, 16); got != (FuelBurn{}) || s.Fuel[0] != 1 {
		t.Fatal("inactive player consumed/published fuel")
	}
	s.Countdown = -1
	if got := s.Advance(true, 16); got != (FuelBurn{}) || s.Countdown != -2 {
		t.Fatal("negative countdown was clamped or treated as zero")
	}
}

func TestOnlyReachedActiveFollowerOldemptyMarksUnsupportedSuccessor(t *testing.T) {
	s := NewSystems([]int16{5, 1, 0, 0, 0, 0}, []bool{true, true, false, false, false, true})
	s.Countdown = 0
	s.Advance(true, 16)
	if s.Fuel[1] != 0 || s.UnsupportedFollowerEmptyBurn != 0 || s.Damage[9] != 0 {
		t.Fatal("newly reached zero/inactive/reserved actor was treated as follower oldempty")
	}
	s.Countdown = 0
	s.Advance(true, 16)
	if s.UnsupportedFollowerEmptyBurn != 2 || s.Damage[9] != 0 {
		t.Fatal("source reached active follower1 oldempty was silently exported as healthy or aliased to player damage")
	}
	s.Fuel[1], s.Countdown = 100, 0
	s.Advance(true, 16)
	if s.UnsupportedFollowerEmptyBurn != 2 {
		t.Fatal("refill cannot erase an unimplemented earlier follower transition")
	}
}
