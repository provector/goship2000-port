package combat

import (
	"reflect"
	"testing"
)

func TestFollowerFuelMessagesAreExplicitLaunchInterpretation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s, _, sys := formationFixture(t)
		s.SetFeedbackEnabled(enabled)
		sys.Fuel = []int16{24, 24, 10, 24, 10}
		sys.Countdown = 0
		sys.AdvanceWithFollowers(true, 16, s.FollowerFuelContext(func() uint16 { t.Fatal("positive fuel consumed RNG"); return 0 }))
		got := events(s, "notification")
		if !enabled {
			if len(got) != 0 {
				t.Fatal("old profile widened", got)
			}
			continue
		}
		if len(got) != 4 {
			t.Fatal(got)
		}
		for i, e := range got {
			if e.Actor != i+1 || e.Notification != []uint16{5, 6, 5, 6}[i] {
				t.Fatal(got)
			}
		}
	}
}

func TestSeveralFirstEngagementsUseSourceThreeHistoryQueue(t *testing.T) {
	s, in := fireFixture(t)
	for a := 2; a < 5; a++ {
		s.init.Entities[a] = s.init.Entities[1]
		s.init.Entities[a].RuntimeRow = a
		s.init.Stores.Ships[a] = s.init.Stores.Ships[1]
		s.init.CrewPerformance[a] = s.init.CrewPerformance[1]
	}
	draws := 0
	in.Random = func() uint16 { draws++; return 127 }
	n := NotificationState{}
	for a := 1; a < 5; a++ {
		if err := s.advanceHealthyFollower(a, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range s.DrainEvents() {
		if e.Kind == "notification" {
			n.Queue(int16(e.Notification), int16(e.Actor), int16(e.Target))
		}
	}
	if draws != 4 || n.Rows != [4][4]byte{{2, 0, 0, 0}, {2, 4, 24, 9}, {2, 3, 24, 9}, {2, 2, 24, 9}} {
		t.Fatal("source overwrite, not four-message promise", draws, n.Rows)
	}
	n.Advance(true, true)
	if n.Display[1][1] != 4 || n.Display[2][1] != 3 || n.Display[3][1] != 2 {
		t.Fatal(n.Display)
	}
	if n.Queue(2, 3, 9) {
		t.Fatal("source rows1/2 duplicate was not suppressed")
	}
	if !n.Queue(2, 2, 9) {
		t.Fatal("source row3 deliberately is not in duplicate scan")
	}
	if !n.Queue(16, 0, 0) || !n.Queue(2, 4, 10) {
		t.Fatal("same-phase objective/changed target were lost")
	}
	if !reflect.DeepEqual(n.Rows[1], [4]byte{2, 4, 24, 10}) || n.Rows[2][0] != 16 {
		t.Fatal(n.Rows)
	}
	// A held/busy/cooldown visit must not promise another engagement notice.
	for a := 1; a < 5; a++ {
		if err := s.advanceHealthyFollower(a, in); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range s.DrainEvents() {
		if e.Kind == "notification" {
			t.Fatal("cooldown fabricated engagement", e)
		}
	}
}
