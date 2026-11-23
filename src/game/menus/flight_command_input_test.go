package menus

import (
	"reflect"
	"testing"
	"time"
)

func TestFlightRepeatReleasePreservesDiscreteOrder(t *testing.T) {
	q := flightCommandInput{Delay: 500 * time.Millisecond, Interval: 100 * time.Millisecond, Capacity: 64}
	held := Actions{FlightHeldValid: true, FlightHeldKeys: [11]bool{true}, FlightCommands: []int{0}}
	q.SampleAt(0, held)
	for i := 0; i <= 5; i++ {
		command, ok := q.NextAt(time.Duration(i) * 100 * time.Millisecond)
		want := i == 0 || i == 5
		if ok != want || (ok && command != 0) {
			t.Fatalf("hand-worked repeat boundary%d: %d/%t", i, command, ok)
		}
	}
	q.SampleAt(550*time.Millisecond, Actions{FlightHeldValid: true, FlightCommands: []int{2, 5}})
	for i, want := range []int{2, 5} {
		if got, ok := q.NextAt(time.Duration(600+i*100) * time.Millisecond); !ok || got != want {
			t.Fatal("release deleted or reordered legitimate command", got, ok, want)
		}
	}
	for i := 8; i < 20; i++ {
		if got, ok := q.NextAt(time.Duration(i) * 100 * time.Millisecond); ok {
			t.Fatal("released collective continued from typematic backlog", got)
		}
	}
}

func TestFlightCommandOverflowAndResetAreExplicit(t *testing.T) {
	q := flightCommandInput{Delay: 500 * time.Millisecond, Interval: 100 * time.Millisecond, Capacity: 2}
	q.SampleAt(0, Actions{FlightHeldValid: true, FlightHeldKeys: [11]bool{true}, FlightCommands: []int{0, 2, 5}})
	q.SampleAt(time.Millisecond, Actions{FlightHeldValid: true})
	if q.Dropped != 1 {
		t.Fatal("bounded queue overflow was not observable", q.Dropped)
	}
	if got, ok := q.NextAt(100 * time.Millisecond); !ok || got != 0 {
		t.Fatal(got, ok)
	}
	if got, ok := q.NextAt(200 * time.Millisecond); !ok || got != 2 {
		t.Fatal(got, ok)
	}
	if got, ok := q.NextAt(time.Second); ok {
		t.Fatal("overflow lost a release transition", got)
	}
	q.SampleAt(2*time.Second, Actions{FlightCommands: []int{3}})
	q.Reset()
	if got, ok := q.NextAt(3 * time.Second); ok || got != 0 || q.Delay != 500*time.Millisecond || q.Capacity != 2 {
		t.Fatal("modal/focus reset leaked queued work or configuration", q)
	}
}

func TestNormalizedTimestampStreamIndependentOfRenderSchedule(t *testing.T) {
	type event struct {
		at time.Duration
		a  Actions
	}
	events := []event{
		{0, Actions{FlightHeldValid: true, FlightHeldKeys: [11]bool{true}, FlightCommands: []int{0}}},
		{310 * time.Millisecond, Actions{FlightHeldValid: true, FlightHeldKeys: [11]bool{true}, FlightCommands: []int{2}}},
		{760 * time.Millisecond, Actions{FlightHeldValid: true}},
		{810 * time.Millisecond, Actions{FlightHeldValid: true, FlightHeldKeys: [11]bool{false, true}, FlightCommands: []int{0x101}, ViewShift: true}},
		{1480 * time.Millisecond, Actions{FlightHeldValid: true}},
	}
	var reference []int
	// Feed captured timestamped events in different display batches. The logical
	// consumer sees only events at/before its boundary, even if a frame batches
	// several events. These expectations do not invoke flight equations.
	for _, fps := range []int{30, 60, 144} {
		q := flightCommandInput{Delay: 500 * time.Millisecond, Interval: 100 * time.Millisecond, Capacity: 64}
		var got []int
		eventIndex, tick := 0, 0
		for frame := 0; frame <= fps*2; frame++ {
			now := time.Duration(frame) * time.Second / time.Duration(fps)
			for eventIndex < len(events) && events[eventIndex].at <= now {
				e := events[eventIndex]
				q.SampleAt(e.at, e.a)
				eventIndex++
			}
			for boundary := time.Duration(tick) * 100 * time.Millisecond; boundary <= now; boundary = time.Duration(tick) * 100 * time.Millisecond {
				command, ok := q.NextAt(boundary)
				if ok {
					got = append(got, tick*1024+command)
				}
				tick++
			}
		}
		if reference == nil {
			reference = got
		} else if !reflect.DeepEqual(reference, got) {
			t.Fatal("render batching changed normalized commands", fps, reference, got)
		}
	}
	// Hand deadlines: press0, toggle400ms, repeats500/600/700, down800ms
	// occurs at900ms, repeats1400ms. Releases exclude800/1500 repeats.
	want := []int{0, 4*1024 + 2, 5 * 1024, 6 * 1024, 7 * 1024, 9*1024 + 0x101, 14*1024 + 0x101}
	if !reflect.DeepEqual(reference, want) {
		t.Fatal("normalized trace differs from independently hand-scheduled boundaries", reference, want)
	}
}

func TestInputLatchPublishesHeldCommandsAndAltTOnlyOnEdge(t *testing.T) {
	var w windowInput
	b := Buttons{AltT: true, FlightKeys: [11]bool{true, false, true}, InputTime: time.Second}
	a := w.ReadViewport(b, Viewport{})
	if !a.NightVisionToggle || !a.FlightHeldValid || a.FlightHeldKeys != b.FlightKeys || a.InputTime != time.Second || len(a.FlightCommands) != 2 {
		t.Fatal(a)
	}
	a = w.ReadViewport(b, Viewport{})
	if a.NightVisionToggle || len(a.FlightCommands) != 0 || !a.FlightHeldKeys[0] {
		t.Fatal("held toggle repeated or collective state disappeared", a)
	}
}

func TestFlightTimelineDropsStallTimeWithoutDelayingRelease(t *testing.T) {
	q := flightCommandTimeline{flightCommandInput: flightCommandInput{Delay: 500 * time.Millisecond, Interval: 100 * time.Millisecond, Capacity: 64}}
	q.Sample(Actions{InputTime: time.Second, FlightHeldValid: true, FlightHeldKeys: [11]bool{true}, FlightCommands: []int{0}, SceneDirections: [4]bool{true}})
	if command, ok := q.NextLogical(); !ok || command != 0 || !q.Directions[0] {
		t.Fatal("first command/directional sample not available at first tick", q)
	}
	q.Sample(Actions{InputTime: 11 * time.Second, FlightHeldValid: true})
	q.NextLogical()
	q.NextLogical()
	q.NextLogical()
	if q.Directions != [4]bool{} || q.held != [2]bool{} {
		t.Fatal("release was retained in dropped ten-second host stall", q)
	}
	for i := 0; i < 10; i++ {
		if command, ok := q.NextLogical(); ok {
			t.Fatal("stall/release caused a repeat burst", command)
		}
	}
}

func TestFlightTimelineIncludesExactClockBoundary(t *testing.T) {
	q := flightCommandTimeline{}
	// Hand division:117600000000000/1193182 =98559984 +1170912/1193182;
	// the first integer-nanosecond triggering boundary is98559985ns.
	q.SampleAt(98559985, Actions{FlightCommands: []int{2}})
	if got, ok := q.NextLogical(); !ok || got != 2 {
		t.Fatal("command at Clock.Advance's exact first boundary delayed", got, ok)
	}
}

// New avionics edges use the same normalized timestamp as collective/axes.
// A101ms radar edge cannot consume RNG at a100ms boundary at any render FPS.
func TestAuxiliaryActionsShareTimestampBoundary(t *testing.T) {
	q := flightCommandInput{Capacity: 64}
	q.SampleAt(101*time.Millisecond, Actions{DefenseCommands: []int{2}, Backspace: true, WaypointNext: true})
	q.NextAt(100 * time.Millisecond)
	if len(q.TakeAux()) != 0 {
		t.Fatal("auxiliary edge applied early")
	}
	q.NextAt(102 * time.Millisecond)
	got := q.TakeAux()
	if len(got) != 3 || got[0] != 2 || got[1] != 4 || got[2] != 5 {
		t.Fatal(got)
	}
	q.NextAt(200 * time.Millisecond)
	if len(q.TakeAux()) != 0 {
		t.Fatal("edge replay")
	}
	q.SampleAt(201*time.Millisecond, Actions{DefenseCommands: []int{0}})
	q.Reset()
	q.NextAt(202 * time.Millisecond)
	if len(q.TakeAux()) != 0 {
		t.Fatal("modal/focus reset retainedaux")
	}
}
