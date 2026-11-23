package navigation

import (
	"encoding/binary"
	"testing"
)

func TestSourceOrderWriters(t *testing.T) {
	for prior := byte(1); prior <= 8; prior++ {
		var r [5]Route
		var active [5]bool
		active[2] = true
		r[2].Raw[1] = prior
		r[2].Raw[5] = 0
		r[0].Raw[1] = 2
		_ = r[2].SetPoint(1, Point{111, 222, 0xbeef})
		before := r[2].Point(1)
		var e [5]OrderEffects
		if prior == 2 {
			r[2].Raw[2] = 2
		} else {
			e = ApplyOrder(&r, active, 2, "h", Point{}, Point{}, Point{}, false)
			if r[2].Raw[1] != 2 || r[2].Raw[2] != prior || e[2].Kind != 0x20 || !e[2].ResetPhase {
				t.Fatalf("hold prior%d: %+v %+v", prior, r[2], e)
			}
		}
		e = ApplyOrder(&r, active, 2, "h", Point{}, Point{}, Point{}, false)
		want := prior
		if prior == 1 {
			want = 4
		}
		if r[2].Raw[1] != want || e[2].Kind != [8]byte{0x24, 0x20, 0x23, 0x24, 0x25, 0x26, 0x26, 0x22}[prior-1] || r[2].Point(1) != before {
			t.Fatal(prior, r[2], e)
		}
	}
	for _, at := range []int{6, 8} {
		for _, old := range []int16{1, 2, 3, 32767, -32768, -1, 0} {
			var r [5]Route
			var a [5]bool
			a[1] = true
			r[1].Raw[1] = 2
			binary.LittleEndian.PutUint16(r[1].Raw[at:], uint16(old))
			key := "a"
			if at == 8 {
				key = "s"
			}
			ApplyOrder(&r, a, 1, key, Point{}, Point{}, Point{}, false)
			want := old + 1
			if want > 3 {
				want = 1
			}
			if got := int16(binary.LittleEndian.Uint16(r[1].Raw[at:])); got != want {
				t.Fatal(at, old, got, want)
			}
		}
	}
}
func TestWeaponsPropagatesXORToLaterModeOneChildrenOnly(t *testing.T) {
	var r [5]Route
	active := [5]bool{true, true, true, true, true}
	for i := range r {
		r[i].Raw[1] = 1
		r[i].Raw[5] = 1
		r[i].Raw[4] = byte(i & 1)
	}
	r[1].Raw[1] = 2
	r[3].Raw[1] = 3
	before := r
	e := ApplyOrder(&r, active, 1, "w", Point{}, Point{}, Point{}, false)
	for i := range r {
		want := before[i].Raw[4]
		change := i == 1 || i == 2 || i == 4
		if change {
			want ^= 1
		}
		if r[i].Raw[4] != want || e[i].ClearCooldown != change {
			t.Fatal(i, r[i].Raw[4], want, e[i])
		}
	}
}
func TestClearAndEligibilityOrderBoundaries(t *testing.T) {
	for _, key := range []string{"l", "d", "g"} {
		var r [5]Route
		var a [5]bool
		a[2] = true
		r[0].Raw[1] = 2
		r[2].Raw[1] = 3
		for n := 1; n <= 3; n++ {
			r[2].SetPoint(n, Point{uint32(n), 77, 0xabcd})
		}
		e := ApplyOrder(&r, a, 2, key, Point{}, Point{}, Point{}, false)
		for n := 1; n <= 3; n++ {
			if got := r[2].Point(n); got != (Point{Tail: 0xabcd}) {
				t.Fatal(key, n, got)
			}
		}
		want := map[string]byte{"l": 0x26, "d": 0x25, "g": 0x24}[key]
		if e[2].Kind != want {
			t.Fatal(key, e)
		}
		if (key == "d") != e[2].ResetPhase {
			t.Fatal("phase", key, e)
		}
	}
	// Mode1 ignores D/G rather than detaching a follower with guessed state.
	for _, key := range []string{"d", "g"} {
		var r [5]Route
		r[1].Raw[1] = 1
		old := r
		ApplyOrder(&r, [5]bool{false, true}, 1, key, Point{}, Point{}, Point{}, false)
		if r != old {
			t.Fatal(key)
		}
	}
}
func TestFollowerFlyFullBackupAndTypedFinish(t *testing.T) {
	var routes [5]Route
	routes[1].Raw[1] = 1
	routes[1].Raw[5] = 0
	for i := 10; i < 70; i++ {
		routes[1].Raw[i] = byte(i)
	}
	g := New(&routes, [5]Point{}, [5]bool{true, true})
	g.Selected = 1
	if err := g.StartEdit(); err == nil {
		t.Fatal("historical readonly changed")
	}
	var finishes []int
	g.SetFollowerOrders(true, func(n int) { finishes = append(finishes, n) })
	old := routes[1]
	if _, err := g.Handle(Input{Key: "f"}); err != nil || !g.Editing {
		t.Fatal(err)
	}
	g.Handle(Input{Escape: true})
	if routes[1] != old || len(finishes) != 0 {
		t.Fatal("cancel did not restore entire row")
	}
	g.Handle(Input{Key: "f"})
	g.Handle(Input{Selector1: true})
	g.Handle(Input{Key: "f"})
	if len(finishes) != 1 || finishes[0] != 1 || routes[1].Index() != 1 || routes[1].Raw[1] != 3 || binary.LittleEndian.Uint16(routes[1].Raw[6:]) != 1 || binary.LittleEndian.Uint16(routes[1].Raw[8:]) != 3 {
		t.Fatal(routes, finishes)
	}
	for n := 1; n <= 3; n++ {
		if routes[1].Point(n).Tail != old.Point(n).Tail {
			t.Fatal("tail", n)
		}
	}
}

func TestGPSAndDirectZeroPriorHoldRemainSourceDistinct(t *testing.T) {
	for _, gps := range []bool{false, true} {
		var r [5]Route
		r[1].Raw[1] = 2
		var a [5]bool
		a[1] = true
		apply := ApplyOrder
		if gps {
			apply = ApplyGPSOrder
		}
		e := apply(&r, a, 1, "h", Point{}, Point{}, Point{}, false)
		want := byte(2)
		if gps {
			want = 0
		}
		if r[1].Raw[1] != want || !e[1].ResetPhase || e[1].ChangeKind {
			t.Fatal(gps, r, e)
		}
	}
}
