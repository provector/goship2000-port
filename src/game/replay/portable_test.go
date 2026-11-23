package replay

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleFileTitleTornWriteAndConflicts(t *testing.T) {
	source := Store{t.TempDir()}
	f := captureBlocks(t, source, strings.Repeat("b", 32), 140)
	if _, e := OpenFile(f.blocks.path); e == nil {
		t.Fatal("private seal playable")
	}
	if e := source.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	if e := source.Rename(f.Header.ID, "Patrol"); e != nil {
		t.Fatal(e)
	}
	data, e := source.Read(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	receiver := Store{t.TempDir()}
	path := filepath.Join(receiver.Root, "Żółty patrol.G2R")
	write := func(path string, b []byte) {
		t.Helper()
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write(path, data)
	row := func() Entry {
		t.Helper()
		rows, e := receiver.List()
		if e != nil || len(rows) != 1 {
			t.Fatalf("list %v %v", rows, e)
		}
		return rows[0]
	}
	if r := row(); r.Title != "Patrol" || r.Error != "" {
		t.Fatal(r)
	}
	g, e := receiver.LoadEntry(row())
	if e != nil {
		t.Fatal(e)
	}
	op, e := g.OperationAt(139)
	if e != nil || !bytes.Equal(op.Payload, blocksOp(139).Payload) {
		t.Fatal(e)
	}
	if e := receiver.RenameEntry(row(), "Second"); e != nil {
		t.Fatal(e)
	}
	path = filepath.Join(receiver.Root, "Second.g2r")
	after, _ := os.ReadFile(path)
	if len(after) != len(data) || !bytes.Equal(data[:len(data)-2*titleSlotSize], after[:len(after)-2*titleSlotSize]) {
		t.Fatal("payload changed")
	}
	// Generation 3 overwrites slot 0. Tear its checksum; generation 2 must win.
	torn := bytes.Clone(after)
	torn[len(torn)-titleSlotSize-1] ^= 1
	write(path, torn)
	if row().Title != "Patrol" {
		t.Fatal("torn update lost previous title")
	}
	duplicate := filepath.Join(receiver.Root, "another.g2r")
	write(duplicate, torn)
	if row().Error != "" {
		t.Fatal("identical duplicate hidden")
	}
	if e := receiver.DeleteEntry(row()); e == nil {
		t.Fatal("ambiguous deletion")
	}
	write(duplicate, after)
	if row().Error == "" {
		t.Fatal("conflict silently merged")
	}
	if _, e := receiver.LoadEntry(row()); e == nil {
		t.Fatal("opened ambiguous identity")
	}
}
func TestRecoveryMigrationCutAndUnscoredTransfer(t *testing.T) {
	f, b, r := recoveryTestFilm(t)
	s := Store{t.TempDir()}
	p, _ := s.path(f.Header.ID)
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	rp, _ := s.RecoveryPath(f.Header.ID)
	if e := WriteRecoveryNew(rp, r); e != nil {
		t.Fatal(e)
	}
	m, e := metadataAt(p)
	if e != nil {
		t.Fatal(e)
	}
	title, _ := json.Marshal(filmTitle{m.revision, "Recovered patrol"})
	if e = os.WriteFile(p+".title.json", title, 0600); e != nil {
		t.Fatal(e)
	}
	status, _ := json.Marshal(UnscoredDisposition{1, f.Header.ID, m.identity, "supplied rejected result"})
	if e = os.WriteFile(p+".unscored.json", status, 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.MigrateCompanions(); e != nil {
		t.Fatal(e)
	}
	// Interruption after activation, before retirement of the verified label.
	if e = os.WriteFile(p+".title.json", title, 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.MigrateCompanions(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(p + ".title.json"); !os.IsNotExist(e) {
		t.Fatal("owned companion retained")
	}
	moved := Store{t.TempDir()}
	data, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(moved.Root, "shared.g2r"), data, 0600); e != nil {
		t.Fatal(e)
	}
	rows, e := moved.List()
	if e != nil || len(rows) != 1 || rows[0].Title != "Recovered patrol" || !rows[0].Unscored {
		t.Fatal(rows, e)
	}
	rec, e := moved.LoadRecovery(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	digest, e := moved.Digest(f.Header.ID)
	if e != nil || digest != BytesDigest(b) {
		t.Fatal("receipt identity changed", e)
	}
	film, e := moved.LoadEntry(rows[0])
	if e != nil {
		t.Fatal(e)
	}
	if e = rec.MatchesDigest(digest, film); e != nil {
		t.Fatal(e)
	}
}
