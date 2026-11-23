package flight

import (
	"reflect"
	"testing"
	"time"
)

func TestSourceStaggerHandTrace(t *testing.T) {
	var c Clock
	// DC starts0; DF first10 then6: DC after each update is0,4,8,2,6,10,4.
	want := []Timing{{10, true, 1}, {6, true, 2}, {6, true, 3}, {6, false, 3}, {6, true, 4}, {6, true, 5}, {6, false, 5}}
	for i, w := range want {
		if got := c.next(); got != w {
			t.Fatalf("update%d got%+v want%+v", i, got, w)
		}
	}
}

func TestLogicalClockRenderPartitionAndPause(t *testing.T) {
	run := func(parts int) []Timing {
		var c Clock
		var out []Timing
		for i := 0; i < parts; i++ {
			// Partition the same integer second, distributing rounding residues.
			d := time.Second*time.Duration(i+1)/time.Duration(parts) - time.Second*time.Duration(i)/time.Duration(parts)
			out = append(out, c.Advance(d, false)...)
		}
		return out
	}
	a, b := run(30), run(144)
	if len(a) != 10 || !reflect.DeepEqual(a, b) {
		t.Fatalf("partitioned source clock%v vs%v", a, b)
	}
	var c Clock
	c.Advance(80*time.Millisecond, false)
	if n := len(c.Advance(time.Hour, true)); n != 0 {
		t.Fatal("pause advanced")
	}
	if n := len(c.Advance(20*time.Millisecond, false)); n != 0 {
		t.Fatal("paused fraction survived")
	}
	if n := len(c.Advance(100*time.Millisecond, false)); n != 1 {
		t.Fatal("resume did not own fresh interval")
	}
}
