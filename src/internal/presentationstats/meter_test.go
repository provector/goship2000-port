package presentationstats

import (
	"testing"
	"time"
)

func TestWholeProcessAndCompletedFrames(t *testing.T) {
	off := Start(false)
	if off != nil {
		t.Fatal("disabled meter active")
	}
	off.Presented()
	base := time.Now()
	m := Meter{start: base, valid: true}
	calls := 0
	ok := true
	read := func() (time.Duration, bool) { calls++; return 2500 * time.Millisecond, ok }
	for i := 1; i <= 20; i++ {
		m.sample(base.Add(time.Duration(i)*50*time.Millisecond), read)
		if i < 20 && calls != 0 {
			t.Fatal("early counter read")
		}
	}
	if calls != 1 || m.Text != "FPS 20.0 | CPU 250%" {
		t.Fatal(m, calls)
	}
	ok = false
	m.sample(base.Add(2*time.Second), read)
	if m.Text != "FPS 1.0 | CPU N/A" {
		t.Fatal(m)
	}
	ok = true
	m.sample(base.Add(3*time.Second), read)
	if m.Text != "FPS 1.0 | CPU N/A" {
		t.Fatal("used invalid baseline", m)
	}
	m.sample(base.Add(4*time.Second), read)
	if m.Text != "FPS 1.0 | CPU 0%" {
		t.Fatal(m)
	}
}
