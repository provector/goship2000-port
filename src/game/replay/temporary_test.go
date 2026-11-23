package replay

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecorderExactBoundsAndIndependentCount(t *testing.T) {
	f := testFilm(strings.Repeat("1", 32))
	s := Store{Root: t.TempDir()}
	r, e := s.BeginTemporary(f)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if r.Limit() != 128<<20 {
		t.Fatal("default", r.Limit())
	}
	o := Operation{Kind: "boundary", Payload: json.RawMessage(`{"Kind":"reset"}`), Checkpoint: map[string]string{"flight": "x"}}
	b, _ := json.Marshal(o)
	// This is an explicit arithmetic boundary fixture; actual writes follow.
	compressed := len(encoder.EncodeAll(append(b, '\n'), nil))
	r.limit = r.bytes + 3*descriptorSize + int64(compressed) + sealReserve
	if e = r.Append(o); e != nil {
		t.Fatal("exact compressed capacity", e)
	}
	if e = r.flushBatch(); e != nil {
		t.Fatal(e)
	}
	if e = r.Append(o); e == nil || !strings.Contains(e.Error(), "compressed film reached") {
		t.Fatal("accepted overrun", e)
	}
	if e = r.Seal(f); e != nil {
		t.Fatal("unable to finalize admitted data", e)
	}
	if f.OperationCount() != 1 {
		t.Fatal(f.OperationCount())
	}

}
func TestSealPreservesOnlyPrefixOnFailure(t *testing.T) {
	f := testFilm(strings.Repeat("2", 32))
	r, e := (Store{Root: t.TempDir()}).BeginTemporary(f)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.buffer.Flush(); e != nil {
		t.Fatal(e)
	}
	prefix, _ := os.ReadFile(r.path)
	// Force rename failure with a nonempty destination directory. All prior
	// staging writes/sync/close run, so this checks the actual transaction boundary.
	os.Mkdir(r.path+".sealed", 0700)
	os.WriteFile(filepath.Join(r.path+".sealed", "keep"), []byte("prior"), 0600)
	if e = r.Seal(f); e == nil {
		t.Fatal("rename failure not reached")
	}
	after, _ := os.ReadFile(r.path)
	if !bytes.HasPrefix(after, prefix) {
		t.Fatal("only prefix truncated")
	}
	r.RecordFailure("seal", 4, 3, e)
	r.RecordFailure("later", 5, 4, os.ErrClosed)
	if r.Failure().Phase != "seal-rename" || r.Failure().Operation != 4 {
		t.Fatal("lost first cause")
	}
	r.Close()
}
func TestBufferedFlushFailureAndSuccessfulSeal(t *testing.T) {
	f := testFilm(strings.Repeat("3", 32))
	r, e := (Store{Root: t.TempDir()}).BeginTemporary(f)
	if e != nil {
		t.Fatal(e)
	}
	r.file.Close() // real closed-descriptor buffered flush, not a fake return value
	if e = r.Seal(f); e == nil || !strings.Contains(e.Error(), "closed") {
		t.Fatal("flush failure", e)
	}
	r.Close()
	f = testFilm(strings.Repeat("4", 32))
	store := Store{Root: t.TempDir()}
	r, e = store.BeginTemporary(f)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Seal(f); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(r.path + ".sealed")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Unmarshal(b); e == nil || !strings.Contains(e.Error(), "private recording") {
		t.Fatal("sealed candidate admitted before publication", e)
	}
	if e = store.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Load(f.Header.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(r.path); !os.IsNotExist(e) {
		t.Fatal("private spool was not atomically moved to sealed name", e)
	}
}
