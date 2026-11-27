package replay

import (
	"encoding/json"
	"strings"
	"testing"
)

// Exercise production append, sealing, random access and full sequential playback.
// Large repeated payload data crosses the former decoded-byte ceiling cheaply on disk.
func TestStreamPastFormerLimits(t *testing.T) {
	f := testFilm(strings.Repeat("c", 32))
	store := Store{t.TempDir()}
	r, err := store.BeginTemporaryWithLimit(f, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	op := Operation{Kind: "boundary", Payload: json.RawMessage(`{"Kind":"reset","pad":"` + strings.Repeat("x", 1080) + `"}`), Checkpoint: map[string]string{"flight": BytesDigest([]byte("capacity"))}}
	const ops = 250003
	for i := 0; i < ops; i++ {
		if err = r.Append(op); err != nil {
			t.Fatal(i, err)
		}
		if i%30 == 29 {
			if err = r.flushBatch(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = r.Seal(f); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFile(f.blocks.path); err == nil {
		t.Fatal("private seal admitted")
	}
	if err = store.PublishLast(f); err != nil {
		t.Fatal(err)
	}
	g, err := OpenFile(f.blocks.path)
	if err != nil {
		t.Fatal(err)
	}
	if g.OperationCount() != ops || g.blocks.decoded <= 256<<20 || g.blocks.manifest.Blocks <= 8196 {
		t.Fatal(g.OperationCount(), g.blocks.manifest.Blocks, g.blocks.decoded)
	}
	for i := 0; i < ops; i++ {
		o, e := g.OperationAt(i)
		if e != nil || o.Checkpoint["flight"] != op.Checkpoint["flight"] {
			t.Fatal(i, e)
		}
	}
	if len(g.Operations) != 0 || len(g.blocks.frames) > indexPageSize || len(g.blocks.rows) > 30 || len(g.blocks.pages) != (g.blocks.manifest.Blocks+255)/256 {
		t.Fatal("unbounded resident directory/rows")
	}
	t.Logf("%d operations, %d frames, %d decoded bytes, %d framed bytes before index, %d page anchors", ops, g.blocks.manifest.Blocks, g.blocks.decoded, r.Bytes(), len(g.blocks.pages))
}
