package audioopl

import (
	"reflect"
	"testing"
)

func TestStreamRetainsChipAcrossTransactions(t *testing.T) {
	// A keyed two-operator voice, with a level change across a block boundary.
	// Expected PCM comes from the original, immutable batch protocol/core, not
	// a second invocation of the stream wrapper under test.
	writes := []Write{{0, 0x20, 1}, {0, 0x23, 1}, {0, 0x40, 0x20}, {0, 0x43, 0}, {0, 0x60, 0xf0}, {0, 0x63, 0xf0}, {0, 0x80, 0x77}, {0, 0x83, 0x77}, {0, 0xa0, 0x98}, {0, 0xb0, 0x31}, {1024, 0x43, 0x10}, {1536, 0xb0, 0x11}}
	want, err := Render(49716, 2048, writes)
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenStream(49716)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var got []int32
	for start := uint64(0); start < 2048; start += 512 {
		var w []Write
		for _, x := range writes {
			if x.Frame >= start && x.Frame < start+512 {
				x.Frame -= start
				w = append(w, x)
			}
		}
		p, e := s.Render(512, w)
		if e != nil {
			t.Fatal(e)
		}
		got = append(got, p...)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("stream chunks reset or change chip state/order")
	}
	if _, e := s.Render(0, nil); e == nil {
		t.Fatal("unbounded/empty transaction accepted")
	}
}
