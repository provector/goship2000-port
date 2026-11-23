package flightcontrol

import "testing"

func TestNativePrecisionDigitalAndCollectivePickup(t *testing.T) {
	s := LiveState{}
	a := Axes{}
	in := LiveInput{Epoch: 1, Axes: [4]int16{37, -63, 0, 81}, Bound: [4]bool{true, true, false, true}}
	c := s.Apply(in, &a, 16, false)
	if a.Y != 37 || a.X != -63 || s.Yaw != 81 || c != 16 {
		t.Fatal(a, s, c)
	}
	in.Digital[1] = [2]bool{false, true}
	s.Apply(in, &a, 16, false)
	if a.X != -47 {
		t.Fatal("digital ramp", a)
	}
	in.Digital[1] = [2]bool{true, true}
	s.Apply(in, &a, 16, false)
	if a.X != 0 {
		t.Fatal("opposites", a)
	}
	in.Digital[1] = [2]bool{}
	s.Apply(in, &a, 16, false)
	if a.X != -63 {
		t.Fatal("analog takeover", a)
	}
	in = LiveInput{Epoch: 2, Absolute: true, Bound: [4]bool{false, false, true}, Lever: 0}
	if c = s.Apply(in, &a, 16, false); c != 16 {
		t.Fatal("parked start lever changed thrust", c)
	}
	in.Lever = 32768
	if c = s.Apply(in, &a, 16, false); c != 16 || !s.Engaged {
		t.Fatal("midpoint pickup", c, s)
	}
	in.Lever = 65535
	if c = s.Apply(in, &a, c, false); c != 31 {
		t.Fatal("max", c)
	}
	if c = s.Apply(in, &a, 20, true); c != 20 || s.Engaged {
		t.Fatal("keyboard ownership", c, s)
	}
	if c = s.Apply(in, &a, c, false); c != 20 {
		t.Fatal("stationary lever undid keyboard", c)
	}
	in.Lever = 41000
	if c = s.Apply(in, &a, c, false); c < 19 || c > 20 || !s.Engaged {
		t.Fatal("pickup", c, s)
	}
	in.Lever = 0
	if c = s.Apply(in, &a, c, false); c != 0 {
		t.Fatal("minimum", c)
	}
	in.Epoch++
	in.Lever = 65535
	if c = s.Apply(in, &a, 11, false); c != 11 {
		t.Fatal("reconnect takeover", c)
	}
}
func TestRelativeCollectiveProportionalAndCenteredHold(t *testing.T) {
	for _, v := range []int16{32, 64, 127, -127} {
		s := LiveState{}
		a := Axes{}
		in := LiveInput{Epoch: 1, Axes: [4]int16{0, 0, v}, Bound: [4]bool{false, false, true}}
		c := int16(16)
		for i := 0; i < 4; i++ {
			c = s.Apply(in, &a, c, false)
		}
		want := int16(16 + int(v)*4/127)
		if c != want {
			t.Fatal(v, c, want)
		}
		in.Axes[2] = 0
		for i := 0; i < 5; i++ {
			if next := s.Apply(in, &a, c, false); next != c {
				t.Fatal("center moved", c, next)
			}
		}
	}
}

func TestAbsoluteParkedMidpointAndJitterCannotTakeOver(t *testing.T) {
	s := LiveState{}
	a := Axes{}
	in := LiveInput{Epoch: 1, Absolute: true, Bound: [4]bool{false, false, true}, Lever: 32768}
	for i := 0; i < 3; i++ {
		if c := s.Apply(in, &a, 16, false); c != 16 || s.Engaged {
			t.Fatal(c, s)
		}
	}
	s.Apply(in, &a, 17, true)
	for _, v := range []uint16{32769, 32767, 32770, 32768} {
		in.Lever = v
		if c := s.Apply(in, &a, 17, false); c != 17 || s.Engaged {
			t.Fatal("jitter reengaged", c, s)
		}
	}
	in.Lever = 35000
	if c := s.Apply(in, &a, 17, false); c != 17 || !s.Engaged {
		t.Fatal("deliberate pickup", c, s)
	}
}
