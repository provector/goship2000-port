package replay

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPointSpoolRollbackAndFailure(t *testing.T) {
	f := testFilm(strings.Repeat("c", 32))
	s := Store{t.TempDir()}
	r, e := s.BeginTemporary(f)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	add := func(i int) {
		t.Helper()
		if e := r.Append(blocksOp(i)); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 71; i++ {
		add(i)
	}
	point, e := r.Point()
	if e != nil {
		t.Fatal(e)
	}
	for cycle := 0; cycle < 2; cycle++ {
		for i := 0; i < 82; i++ {
			add(i + 1000)
		}
		read := point.Reader()
		for i := 0; i < 71; i++ {
			o, e := read.Next()
			if e != nil || !reflect.DeepEqual(o, blocksOp(i)) {
				t.Fatal(i, e)
			}
		}
		if _, e = read.Next(); e != io.EOF {
			t.Fatal(e)
		}
		if e = point.Restore(r); e != nil {
			t.Fatal(e)
		}
		if r.Count() != 71 || !bytes.Equal(r.pending.Bytes(), point.pending) {
			t.Fatal("spool state")
		}
	}
	add(200)
	prior, e := os.ReadFile(r.path)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.file.Close(); e != nil {
		t.Fatal(e)
	}
	if e = point.Restore(r); e == nil {
		t.Fatal("closed writer accepted")
	}
	after, e := os.ReadFile(r.path)
	if e != nil || !bytes.Equal(after, prior) || r.Count() != 72 {
		t.Fatal("failure discarded continuation", e)
	}
}

func TestFailedPointTruncatePreservesAbandonedTailForRetry(t *testing.T) {
	f := testFilm(strings.Repeat("d", 32))
	w, err := (Store{t.TempDir()}).BeginTemporary(f)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	add := func(n int) {
		t.Helper()
		if err := w.Append(blocksOp(n)); err != nil {
			t.Fatal(err)
		}
	}
	add(1)
	point, err := w.Point()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		add(i + 2)
	}
	if w.bytes <= point.bytes {
		t.Fatal("fixture needs completed frames beyond the bookmark")
	}
	before, err := os.ReadFile(w.path)
	if err != nil {
		t.Fatal(err)
	}
	pending := bytes.Clone(w.pending.Bytes())
	count := w.Count()
	ro, err := os.Open(w.path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	writer := w.file
	w.file = ro
	if _, err := ro.Seek(w.bytes, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := point.Restore(w); err == nil {
		t.Fatal("truncate on read-only descriptor succeeded")
	}
	pos, err := ro.Seek(0, io.SeekCurrent)
	if err != nil || pos != w.bytes {
		t.Fatal("write position lost", pos, err)
	}
	after, err := os.ReadFile(w.path)
	if err != nil || !bytes.Equal(before, after) || !bytes.Equal(pending, w.pending.Bytes()) || w.Count() != count {
		t.Fatal("failed restore changed the continuation", err)
	}
	w.file = writer
	add(400)
	if err := point.Restore(w); err != nil || w.Count() != 1 {
		t.Fatal("retry failed", err)
	}
}
