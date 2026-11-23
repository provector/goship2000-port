package flight

import (
	"reflect"
	"testing"
)

func TestNonplayerFuelNoticeBeforeDecrementAndActualPrefixOrder(t *testing.T) {
	for _, e0 := range []bool{false, true} {
		s := NewSystems([]int16{24, 24, 10, 24, 10}, nil)
		s.Countdown = 0
		var notices [][3]int
		ctx := &FollowerFuelContext{Read: func(a int) FuelActor { return FuelActor{Prefix: a < 4, Control: 1, Damage603: 1} }, Notify: func(a int, k uint16) { notices = append(notices, [3]int{a, int(k), int(s.Fuel[a])}) }}
		s.AdvanceWithFollowers(e0, 16, ctx)
		var want [][3]int
		if e0 {
			want = [][3]int{{1, 5, 24}, {2, 6, 10}, {3, 5, 24}}
		}
		if !reflect.DeepEqual(notices, want) {
			t.Fatal(e0, notices, want)
		}
		if e0 && (!reflect.DeepEqual(s.Fuel, []int16{23, 22, 8, 22, 10})) {
			t.Fatal("source damage decrement/prefix", s.Fuel)
		}
	}
	for _, fuel := range []int16{-1, 0, 1, 9, 10, 11, 23, 24, 25} {
		s := NewSystems([]int16{100, fuel}, nil)
		s.Countdown = 0
		var got []uint16
		ctx := &FollowerFuelContext{Read: func(a int) FuelActor { return FuelActor{Prefix: true, Control: 1} }, Notify: func(a int, k uint16) { got = append(got, k) }}
		s.AdvanceWithFollowers(true, 16, ctx)
		if (len(got) == 1) != (fuel == 10 || fuel == 24) {
			t.Fatal(fuel, got)
		}
	}
}
