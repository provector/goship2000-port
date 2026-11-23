package avionics

import (
	"reflect"
	"testing"
)

// Supplied revalidation-entry effects exercise the production retry owner and
// selector. They are not a naturally observed ninth-visit terrain encounter.
func retryFixture(n int) *State {
	s := New()
	for i := 0; i < n; i++ {
		s.Candidates[i] = Candidate{byte(i + 1), 100, 0}
		s.Contacts[i+1] = Contact{Flags: 2}
	}
	s.Target = 2
	return s
}

func TestTargetRepeatedFullStateAndRealProgress(t *testing.T) {
	for _, modern := range []bool{false, true} {
		s := retryFixture(1)
		calls := 0
		err := s.revalidateSelected(Input{ProgressAwareTargeting: modern}, func(Input) (bool, error) {
			calls++
			s.BodyQuery.Status = 1
			s.Cursor = 1
			return false, nil
		})
		want := 8
		if modern {
			want = 2
		}
		if err != nil || calls != want || s.Target != 0 || s.QueryRetryBoundHits != 1 || s.BodyQuery.Status != 1 || s.Contacts[1].Flags != 2 {
			t.Fatal(modern, calls, s, err)
		}
	}
	s := retryFixture(1)
	calls := 0
	if err := s.revalidateSelected(Input{ProgressAwareTargeting: true}, func(Input) (bool, error) {
		calls++
		if calls == 1 {
			s.BodyQuery.Status = 0x101
			s.Contacts[1].Kind = 0x10
			return false, nil
		}
		s.HeadQuery.Status = 1
		return true, nil
	}); err != nil || calls != 2 || s.Target != 2 {
		t.Fatal(calls, s, err)
	}
}

func TestTargetLateProgressUsesRealSelector(t *testing.T) {
	for _, modern := range []bool{false, true} {
		s := retryFixture(8)
		s.Target = 10
		s.Contacts[9].Flags = 2
		visits := []uint16{}
		err := s.revalidateSelected(Input{ProgressAwareTargeting: modern}, func(Input) (bool, error) {
			visits = append(visits, s.Target)
			row := s.Target - 1
			s.BodyQuery.Status = 1
			if row == 8 {
				s.HeadQuery.Status = 1
				return true, nil
			}
			blocked(&s.Contacts[row])
			s.HeadQuery.Status = 0
			s.Cursor = 0
			s.Target, s.Cursor = s.selectTarget(0)
			return false, nil
		})
		want := uint16(0)
		count := 8
		if modern {
			want = 9
			count = 9
		}
		if err != nil || s.Target != want || len(visits) != count {
			t.Fatal(modern, visits, s.Target, err)
		}
		for _, row := range []int{1, 2, 3, 4, 5, 6, 7, 9} {
			if s.Contacts[row].Flags != 1 {
				t.Fatal("observation erased", row)
			}
		}
	}
}

func TestTargetWatchdogExactDomainAndImmutableKeys(t *testing.T) {
	for n := 0; n <= 8; n++ {
		s := retryFixture(n)
		s.Target = 10
		calls := 0
		err := s.revalidateSelected(Input{ProgressAwareTargeting: true}, func(Input) (bool, error) {
			calls++
			s.HeadQuery.Status++
			return false, nil // intentionally outside proved normal transitions
		})
		if err != nil || calls != 2*(n+1)+2 || s.Target != 0 || s.QueryRetryBoundHits != 1 {
			t.Fatal(n, calls, err)
		}
	}
	// Full retained words and full flags/kinds are stable values in visited
	// keys, including the initial row outside the cache. A target-ID set would
	// stop on the second visit and a mutable key would stop incorrectly.
	s := retryFixture(1)
	s.Target = 10
	calls := 0
	if err := s.revalidateSelected(Input{ProgressAwareTargeting: true}, func(Input) (bool, error) {
		calls++
		switch calls {
		case 1:
			s.Contacts[9].Flags = 0x80
		case 2:
			s.Contacts[9].Kind = 0x80
		case 3:
			s.HeadQuery.Status = 0x100
		default:
			return true, nil
		}
		return false, nil
	}); err != nil || calls != 4 || s.Target != 10 {
		t.Fatal(calls, s, err)
	}
}

func TestTargetActualAdvanceOrdinaryVisibilityAndTail(t *testing.T) {
	for _, height := range []int16{0, 200} {
		for _, mast := range []bool{false, true} {
			in, n := contactFixture(t, height)
			in.Mast = mast
			old := New()
			if err := old.Advance(in); err != nil {
				t.Fatal(err)
			}
			if *n != 5 {
				t.Fatal(*n)
			}
			in, n = contactFixture(t, height)
			in.Mast = mast
			in.ProgressAwareTargeting = true
			current := New()
			if err := current.Advance(in); err != nil {
				t.Fatal(err)
			}
			if *n != 5 || !reflect.DeepEqual(old, current) {
				t.Fatal(height, mast, *n, old, current)
			}
		}
	}
	// A no-candidate phase still owns one five-word tail, not a search error.
	in, n := contactFixture(t, 0)
	in.ProgressAwareTargeting = true
	in.Entities[1].Present = false
	s := New()
	if err := s.Advance(in); err != nil || s.Target != 0 || *n != 5 {
		t.Fatal(s.Target, *n, err)
	}
}
