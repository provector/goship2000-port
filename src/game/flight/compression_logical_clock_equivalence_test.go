package flight

import (
	"reflect"
	"testing"
	"time"
)

func TestCompressionLogicalClockEquivalence(t *testing.T) {
	var reference []Timing
	for _, rate := range []int{1, 2, 4, 6, 8, 10, 12, 14, 16} {
		var c Clock
		var got []Timing
		// Same exactly represented logical second; partition host credit independently.
		total := time.Second / time.Duration(rate)
		for i := 0; i < 144; i++ {
			d := total*time.Duration(i+1)/144 - total*time.Duration(i)/144
			c.Schedule(d, rate)
			for q, ok := c.PopScheduled(); ok; q, ok = c.PopScheduled() {
				got = append(got, q)
			}
		}
		if rate == 1 {
			reference = got
		}
		if !reflect.DeepEqual(reference, got) {
			t.Fatalf("rate%d %v != %v", rate, got, reference)
		}
	}
}
func TestCancellationCommitsOnlyReachedClockSteps(t *testing.T) {
	var c Clock
	c.Schedule(250*time.Millisecond, 16)
	a, _ := c.PopScheduled()
	b, _ := c.PopScheduled()
	c.ResetElapsed()
	c.Schedule(100*time.Millisecond, 1)
	d, ok := c.PopScheduled()
	if !ok || a != (Timing{10, true, 1}) || b != (Timing{6, true, 2}) || d != (Timing{6, true, 3}) {
		t.Fatal(a, b, d, ok)
	}
	var stall Clock
	stall.Schedule(time.Hour, 16)
	n := 0
	for _, ok := stall.PopScheduled(); ok; _, ok = stall.PopScheduled() {
		n++
	}
	if n != 16 {
		t.Fatal("unbounded compressed stall", n)
	}
}
func TestBracketAndSourceLimiter(t *testing.T) {
	var c Compression
	for _, v := range []int16{2, 4, 6, 8, 10, 12, 14, 16, 16} {
		c.Increase()
		if c.Request != v {
			t.Fatal(c)
		}
	}
	for _, v := range []int16{0, 2, 4, 6, 16} {
		c.Request = v
		c.Limit()
		want := v
		if v > 4 {
			want = 0
		}
		if c.Request != want {
			t.Fatal(v, c)
		}
	}
	c.Reset()
	if c.Rate() != 1 {
		t.Fatal(c)
	}
}
