package replay

import (
	"strings"
	"testing"
)

func FuzzPublishedRecording(f *testing.F) {
	film := testFilm(strings.Repeat("c", 32))
	film.Operations = []Operation{blocksOp(0)}
	valid, e := Marshal(film)
	if e != nil {
		f.Fatal(e)
	}
	f.Add(valid)
	f.Add(valid[:len(valid)/2])
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 128<<10 {
			return
		}
		doc, e := Unmarshal(b)
		if e != nil {
			return
		}
		if e = doc.Validate(); e != nil {
			t.Fatal("reader admitted an invalid film", e)
		}
		if doc.OperationCount() > 512 {
			return
		}
		again, e := Marshal(doc)
		if e != nil {
			t.Fatal(e)
		}
		out, e := Unmarshal(again)
		if e != nil || out.Header.ID != doc.Header.ID || out.OperationCount() != doc.OperationCount() {
			t.Fatal("detached replay round trip", e)
		}
	})
}
