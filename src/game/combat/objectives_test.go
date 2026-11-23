package combat

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestCountObjectiveOriginalBranches(t *testing.T) {
	// Source file mapping is section1 d8b0+IP. Check actual signed branch,
	// clear-before-event, speech bit and all three result-call operands.
	b, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if os.IsNotExist(err) {
		t.Skip("optional original byte witness unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[int][]byte{
		0x5d34: {0xe9, 0x33, 0x06},
		0x6386: {0x26, 0x39, 0x06, 0xed, 0x0a, 0x7c, 0x2a},
		0x6391: {0x26, 0xc7, 0x06, 0xe8, 0x15, 0x00, 0x00, 0xf6, 0x06, 0xde, 0x00, 0x80},
		0x63a9: {0x6a, 0x00, 0x6a, 0x00, 0x6a, 0x04, 0x9a, 0xc2, 0x0b, 0x00, 0x00},
		0x63c6: {0x26, 0xff, 0x0e, 0x3a, 0x16, 0x74, 0x03, 0xe9, 0xce, 0x00},
		0x64b5: {0xe9, 0x20, 0x06},
		0x6af4: {0x26, 0x39, 0x06, 0xef, 0x0a, 0x7c, 0x2a},
		0x6aff: {0x26, 0xc7, 0x06, 0x26, 0x16, 0x00, 0x00, 0xf6, 0x06, 0xde, 0x00, 0x80},
		0x6b17: {0x6a, 0x00, 0x6a, 0x00, 0x6a, 0x05, 0x9a, 0xc2, 0x0b, 0x00, 0x00},
	} {
		if !bytes.Equal(b[0xd8b0+ip:][:len(want)], want) {
			t.Fatalf("section1:%04x operand mismatch", ip)
		}
	}
}

func TestCountObjectivesLogicalCompletionOrderingAndNoRepeat(t *testing.T) {
	s, in := fixture(t, 2)
	s.Primary = Objective{DisplayKind: 3, Required: 2, Destroyed: 2}
	s.Secondary = Objective{DisplayKind: 3, Required: 1, Destroyed: 1}
	s.Results[3] = Result{Sequence: 254, Kind: 1, Actor: 3, Target: 4, Timestamp: 17}
	in.Phase = 0x4567
	in.SpeechEnabled = true
	in.SecondaryCountReady = true // Supplied reached6ad8 arm, not an invented timer default.
	in.Random = func() uint16 { t.Fatal("objective completion must not consume RNG"); return 0 }
	s.AdvanceObjectives(in)
	if s.Primary.DisplayKind != 0 || s.Secondary.DisplayKind != 0 || !s.Primary.Achieved || !s.Secondary.Achieved {
		t.Fatal("completion state not published")
	}
	if s.Results[2] != (Result{Sequence: 255, Kind: 4, Timestamp: 0x4567}) || s.Results[3] != (Result{Sequence: 0, Kind: 5, Timestamp: 0x4567}) {
		t.Fatalf("ordered rolling results%+v", s.Results)
	}
	sounds := events(s, "sound")
	if len(sounds) != 2 || sounds[0].Command != 0x3e || sounds[1].Command != 0x3f {
		t.Fatalf("completion speech%+v", sounds)
	}
	completed := events(s, "objective-complete")
	if len(completed) != 2 || completed[0].Value != 4 || completed[1].Value != 5 {
		t.Fatalf("completion events%+v", completed)
	}
	priorResults, priorEvents := s.Results, append([]Event(nil), s.Events...)
	s.AdvanceObjectives(in)
	if s.Results != priorResults || !reflect.DeepEqual(s.Events, priorEvents) {
		t.Fatal("objective completed more than once")
	}
	// A later OBJECT10 hit still creates its destruction record, but the now-zero
	// objective kind must leave its counter and threshold notification unchanged.
	s.init.Entities[1].Object.Raw[29], s.init.Entities[1].Object.Raw[30] = 1, 0
	in.Random = func() uint16 { return 2 }
	if err := s.ApplyStationaryHit(1, 0, 2, 1, in); err != nil {
		t.Fatal(err)
	}
	if s.Primary.Destroyed != 2 || len(events(s, "notification")) != 0 {
		t.Fatal("later hit recounted completed objective")
	}
}

func TestSecondaryCompletionRequiresKnownCurrentGoalBranch(t *testing.T) {
	s, in := fixture(t, 2)
	s.Secondary = Objective{DisplayKind: 3, Required: 1, Destroyed: 1}
	s.AdvanceObjectives(in)
	if s.Secondary.DisplayKind != 3 || len(s.Events) != 0 {
		t.Fatal("unknown timer/retask path assumed eligible")
	}
	in.SecondaryCountReady = true
	s.AdvanceObjectives(in)
	if s.Secondary.DisplayKind != 0 || s.Results[3].Kind != 5 {
		t.Fatal("supplied source count arm not consumed")
	}
}

func TestCountObjectivesSignedThresholdModeAndSpeechGates(t *testing.T) {
	for _, tc := range []struct {
		kind                int
		destroyed, required int16
		want                bool
	}{
		{3, 1, 2, false}, {3, 2, 2, true}, {3, 3, 2, true},
		{0, 2, 2, false}, {1, 2, 2, false}, {2, 2, 2, false}, {4, 2, 2, false},
		{3, -32768, 32767, false}, {3, 32767, -32768, true}, {3, 0, 0, true},
	} {
		s, in := fixture(t, 2)
		s.Primary = Objective{DisplayKind: tc.kind, Destroyed: tc.destroyed, Required: tc.required}
		in.SpeechEnabled = false
		in.Random = func() uint16 { t.Fatal("no objective RNG"); return 0 }
		s.AdvanceObjectives(in)
		if (s.Primary.Achieved) != tc.want || len(events(s, "sound")) != 0 {
			t.Fatalf("case%+v state%+v", tc, s.Primary)
		}
		if (len(events(s, "objective-complete")) == 1) != tc.want {
			t.Fatalf("case%+v completion event", tc)
		}
	}
}
