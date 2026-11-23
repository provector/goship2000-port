package menus

import (
	"reflect"
	"testing"
	"time"
)

func TestNativeJammerMakeRepeatAndRelease(t *testing.T) {
	var latch inputLatch
	q := flightCommandInput{Delay: 500 * time.Millisecond, Interval: 100 * time.Millisecond, Capacity: 64}
	press := latch.Read(Buttons{DefenseKeys: [4]bool{false, false, true, false}})
	if !press.DefenseHeldValid || press.DefenseHeld != [2]bool{true, false} || !reflect.DeepEqual(press.DefenseCommands, []int{2}) {
		t.Fatal(press)
	}
	q.SampleAt(0, press)
	var got []int
	for tick := 0; tick <= 8; tick++ {
		at := time.Duration(tick) * 100 * time.Millisecond
		if tick == 7 {
			q.SampleAt(650*time.Millisecond, latch.Read(Buttons{}))
		}
		q.NextAt(at)
		for _, action := range q.TakeAux() {
			got = append(got, tick*10+action)
		}
	}
	if !reflect.DeepEqual(got, []int{2, 52, 62}) {
		t.Fatal("press, delay500,repeat600,release650", got)
	}
	q.SampleAt(time.Second, latch.Read(Buttons{DefenseKeys: [4]bool{false, false, false, true}}))
	q.NextAt(time.Second)
	if got := q.TakeAux(); !reflect.DeepEqual(got, []int{3}) {
		t.Fatal("IR uses independent physical key", got)
	}
	q.Reset()
	q.NextAt(3 * time.Second)
	if got := q.TakeAux(); len(got) != 0 {
		t.Fatal("modal/focus reset retained jammer repeats", got)
	}
}

func TestJammerRepeatsUseTimestampNotDisplayFrequency(t *testing.T) {
	for _, fps := range []int{30, 60, 144} {
		q := flightCommandInput{Delay: 500 * time.Millisecond, Interval: 100 * time.Millisecond, Capacity: 64}
		q.SampleAt(0, Actions{DefenseCommands: []int{3}, DefenseHeldValid: true, DefenseHeld: [2]bool{false, true}})
		q.SampleAt(750*time.Millisecond, Actions{DefenseHeldValid: true})
		var got []int
		tick := 0
		for frame := 0; frame <= fps; frame++ {
			now := time.Duration(frame) * time.Second / time.Duration(fps)
			for time.Duration(tick)*100*time.Millisecond <= now {
				q.NextAt(time.Duration(tick) * 100 * time.Millisecond)
				for _, action := range q.TakeAux() {
					got = append(got, tick*10+action)
				}
				tick++
			}
		}
		if !reflect.DeepEqual(got, []int{3, 53, 63, 73}) {
			t.Fatal(fps, got)
		}
	}
}
