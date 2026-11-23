package flight

import (
	"fmt"
	"reflect"
	"testing"
)

func TestWaterRoughContactAndHoverSettings(t *testing.T) {
	p, base := sourceFixture(t)
	for _, switches := range []uint16{0, 4, 0x20, 0x24, 0x400, 0x404, 0x420, 0x424} {
		for _, surface := range []struct {
			name   string
			height int16
		}{{"water", -1}, {"land", 0}, {"deck", 360}} {
			t.Run(fmt.Sprintf("%04x/%s", switches, surface.name), func(t *testing.T) {
				q := base
				q.Height = func(Pose) (int16, error) { return surface.height, nil }
				s := State{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: surface.height + 2}, Rotor: 1, Collective: 0, Vertical: -4, RadarAltitude: 0}
				if err := Step(&s, p, Input{DF: 6, E0: true, Phase: 2, Switches: switches, Random: func() uint16 { return 0 }}, q); err != nil {
					t.Fatal(err)
				}
				if surface.height < 0 {
					if s.Terminal == nil || s.Terminal.SourceCode != 8 || s.Terminal.Status != 0x10 || s.ValidContact {
						t.Fatal("negative equal support must reach original rough contact", s)
					}
				} else if s.Terminal != nil || !s.ValidContact {
					t.Fatal("gentle flat contact rejected", s)
				}
			})
		}
	}
	q := base
	q.Height = func(Pose) (int16, error) { return -1, nil }
	s := State{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 1}, Rotor: 1, Collective: 8, RadarAltitude: 0}
	if err := Step(&s, p, Input{DF: 6, Phase: 2}, q); err != nil {
		t.Fatal(err)
	}
	if s.Terminal != nil || s.ValidContact {
		t.Fatal("water highest+2 equality is low hover, not touchdown", s)
	}
	s = State{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 64}, Rotor: 1, Collective: 8, Hover: 1, HoverDelay: 16, LowLimit: 128, RadarAltitude: 63}
	if err := Step(&s, p, Input{DF: 6, E0: true, Phase: 2, Switches: 0x20}, q); err != nil {
		t.Fatal(err)
	}
	if s.Terminal != nil || s.Pose.Height <= 1 {
		t.Fatal("source auto-hover must preserve hover rescue", s)
	}
}

func TestShakeDisplayCallbackPreservesStateAndFailureAtomicity(t *testing.T) {
	p, q := sourceFixture(t)
	a := State{Pose: Pose{H0: 0x40000, H1: 0x40000, Height: 500}, Rotor: 1, Collective: 8, Shake: 16, RadarAltitude: 498}
	b := a
	calls := []int16{}
	in := Input{DF: 6, E0: true, Phase: 2}
	if err := Step(&a, p, in, q); err != nil {
		t.Fatal(err)
	}
	in.ShakeDisplay = func(n int16) { calls = append(calls, n) }
	if err := Step(&b, p, in, q); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(calls, []int16{15}) {
		t.Fatal("display callback changed authoritative state", a, b, calls)
	}
	in.E0 = false
	if err := Step(&b, p, in, q); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatal("non-E0 visit advanced display-start latch", calls)
	}
	before := b
	in.E0 = true
	q.Height = func(Pose) (int16, error) { return 0, fmt.Errorf("supplied height failure") }
	if err := Step(&b, p, in, q); err == nil {
		t.Fatal("query failure lost")
	}
	if !reflect.DeepEqual(before, b) || len(calls) != 1 {
		t.Fatal("failed flight transaction published shake")
	}
}
