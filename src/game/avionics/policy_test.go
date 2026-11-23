package avionics

import "testing"

func TestDisableAutonomyKeepsFollowingSensorTailButSkipsIndependent(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		s, in, assigned := tailFixture(t)
		in.Entities[2].Object[10] = 0 // One actual assigned actor; no synthetic sibling decisions.
		in.Entities[1].Control[1] = 2
		s.Contacts[10].Flags = 1
		in.DisableAutonomy = disabled
		if e := s.advanceActorTail(in); e != nil {
			t.Fatal(e)
		}
		if (assigned[1] != 0) == disabled {
			t.Fatalf("independent policy=%v assignment=%d", disabled, assigned[1])
		}
	}
	s, in, assigned := tailFixture(t)
	in.DisableAutonomy = true
	in.Assets = nil // Following handoff must preserve mast observer, no independent ray.
	if e := s.advanceActorTail(in); e != nil {
		t.Fatal(e)
	}
	if assigned[1] != 11 || s.Contacts[10].Flags != 0xfa {
		t.Fatal("shared sensor state changed", *assigned, s.Contacts[10])
	}
}

func TestPolicyDoesNotSkipUnconditionalTailDraws(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		s, in, _ := tailFixture(t)
		in.DisableAutonomy = disabled
		n := 0
		in.Random = func() uint16 { n++; return 0 }
		if e := s.Advance(in); e != nil {
			t.Fatal(e)
		}
		if n != 5 {
			t.Fatalf("policy=%v unconditional shared draws=%d", disabled, n)
		}
	}
}
