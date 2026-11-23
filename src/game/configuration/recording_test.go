package configuration

import "testing"

func TestRecordingLimitPolicy(t *testing.T) {
	for _, n := range []string{"1", "64", "128", "256", "1024"} {
		l, e := ParseRecordingLimit(n)
		if e != nil || l.Validate() != nil {
			t.Fatal(n, l, e)
		}
	}
	for _, n := range []string{"0", "-1", "1025", "1.5", "", "18446744073709551616", "9223372036854775807"} {
		if _, e := ParseRecordingLimit(n); e == nil {
			t.Fatal("admitted", n)
		}
	}
	if RecordingLimit(0).Effective() != 134217728 || ReplayReadMaxBytes != 1<<30 {
		t.Fatal("policy")
	}
}
