package avionics

import "testing"

func TestContactOrderTailHasSeparateInvocation(t *testing.T) {
	for _, c := range []struct {
		k, f byte
		want bool
	}{{0x15, 0, false}, {0x15, 0x10, true}, {0x15, 0x20, true}, {0x15, 0x30, true}, {0x14, 0x30, false}, {0x15, 0x40, false}} {
		s := New()
		s.ReachedContactOrderTail(c.k, c.f)
		if s.CompressionLimited != c.want {
			t.Fatal(c)
		}
	}
}
