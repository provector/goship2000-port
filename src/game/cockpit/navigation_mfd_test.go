package cockpit

import "testing"

func TestSourceNavigationFormatting(t *testing.T) {
	for _, q := range []struct {
		v uint16
		s string
	}{{12345, "12.3km"}, {99, "0.0km"}, {65535, "65.5km"}} {
		if s := RangeText(q.v); s != q.s {
			t.Fatal(q, s)
		}
	}
	if s := BearingText(89 * 183); s != "089`" {
		t.Fatal(s)
	}
}
