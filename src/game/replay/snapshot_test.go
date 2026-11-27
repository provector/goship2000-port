package replay

import (
	"strings"
	"testing"
)

// A debrief assigns the final earned score after exposing the sealed
// diagnostic view. Each score must stay paired with its own summary.
func TestSealedSnapshotOwnsResultAndCachedOperation(t *testing.T) {
	f := captureBlocks(t, Store{Root: t.TempDir()}, strings.Repeat("a", 32), 1)
	f.End.Summary = &Summary{}
	snapshot, err := f.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	f.End.Score = 44
	f.End.Summary.Score = 44
	if err = snapshot.Validate(); err != nil || snapshot.End.Score != 0 || snapshot.End.Summary.Score != 0 {
		t.Fatal("snapshot followed later debrief", err)
	}
	snapshot.Operations[0].Checkpoint["flight"] = "changed"
	op, err := f.OperationAt(0)
	if err != nil || op.Checkpoint["flight"] != BytesDigest([]byte("retained")) {
		t.Fatal("snapshot mutated cached operation", err)
	}
}
