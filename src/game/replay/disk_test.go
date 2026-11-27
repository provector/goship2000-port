package replay

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt in to the 1 GiB of temporary disk traffic. All bytes pass through the
// normal compressor, seal, immutable publication and authenticated block reader.
func TestFullStoredBudget(t *testing.T) {
	if os.Getenv("GS2000_T11_DISK") != "1" {
		t.Skip("requires 1 GiB disposable disk")
	}
	s := Store{t.TempDir()}
	f := testFilm(strings.Repeat("a", 32))
	r, e := s.BeginTemporaryWithLimit(f, 1024<<20)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	data := make([]byte, 1<<20)
	rand.New(rand.NewSource(71)).Read(data)
	payload, _ := json.Marshal(base64.StdEncoding.EncodeToString(data))
	op := Operation{Kind: "step", Payload: payload, Checkpoint: map[string]string{"flight": BytesDigest([]byte("supplied"))}}
	for r.Bytes() < 1021<<20 {
		if err := r.Append(op); err != nil {
			t.Fatal(err)
		}
	}
	if e = r.Seal(f); e != nil {
		t.Fatal(e)
	}
	f.End.Score = 2
	if e = s.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	path, _ := s.resolved(f.Header.ID, true)
	st, _ := os.Stat(path)
	if st.Size() <= 1020<<20 || st.Size() > 1024<<20 {
		t.Fatal(st.Size())
	}
	g, e := OpenFile(path)
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range []int{0, g.OperationCount() / 2, g.OperationCount() - 1} {
		row, e := g.OperationAt(i)
		if e != nil || !bytes.Equal(row.Payload, op.Payload) {
			t.Fatal(i, e)
		}
	}
	t.Logf("%d accepted frames; published %d of %d bytes; index %d bytes; verified first/middle/last", r.Count(), st.Size(), r.Limit(), g.blocks.manifest.Blocks*descriptorSize)
}

func TestStoredLimitRetainsPrefixAndPriorLast(t *testing.T) {
	s := Store{t.TempDir()}
	prior := captureBlocks(t, s, strings.Repeat("e", 32), 1)
	if err := s.PublishLast(prior); err != nil {
		t.Fatal(err)
	}
	f := testFilm(strings.Repeat("a", 32))
	r, e := s.BeginTemporaryWithLimit(f, 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	data := make([]byte, 90<<10)
	rand.New(rand.NewSource(71)).Read(data)
	payload, _ := json.Marshal(base64.StdEncoding.EncodeToString(data))
	op := Operation{Kind: "step", Payload: payload, Checkpoint: map[string]string{"flight": BytesDigest([]byte("supplied"))}}
	for i := 0; i < 25; i++ {
		if e = r.Append(op); e != nil {
			break
		}
	}
	if e == nil || !strings.Contains(e.Error(), "compressed film reached") {
		t.Fatal(e)
	}
	r.RecordFailure("append", uint64(r.Count()+1), 0, e)
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	if r.Seal(f) == nil {
		t.Fatal("failed capture became complete")
	}
	_, ops, n, _ := InspectPrefix(r.path)
	if ops != r.Count() || ops == 0 || n != r.Bytes() {
		t.Fatal(ops, r.Count(), n, r.Bytes())
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 || rows[0].ID != prior.Header.ID {
		t.Fatal(rows, e)
	}
}

func TestOversizedStartNeverWritesPastBudget(t *testing.T) {
	s := Store{t.TempDir()}
	f := testFilm(strings.Repeat("b", 32))
	raw := make([]byte, 1200<<10)
	rand.New(rand.NewSource(105)).Read(raw)
	f.Launch, _ = json.Marshal(base64.StdEncoding.EncodeToString(raw))
	if _, e := s.BeginTemporaryWithLimit(f, 1<<20); e == nil || !strings.Contains(e.Error(), "compressed launch") {
		t.Fatal(e)
	}
	st, e := os.Stat(filepath.Join(s.Root, ".film-"+f.Header.ID+".tmp"))
	if e != nil || st.Size() != 0 {
		t.Fatal("rejected launch wrote payload", st, e)
	}
}
