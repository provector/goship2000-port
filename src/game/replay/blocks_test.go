package replay

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func blocksOp(i int) Operation {
	b, _ := json.Marshal(struct {
		Index int
		Data  string
	}{i, strings.Repeat("input", 400)})
	return Operation{Kind: "boundary", Payload: b, Checkpoint: map[string]string{"flight": BytesDigest([]byte("retained"))}}
}
func captureBlocks(t *testing.T, s Store, id string, n int) *Film {
	t.Helper()
	f := testFilm(id)
	r, e := s.BeginTemporary(f)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	for i := 0; i < n; i++ {
		if e = r.Append(blocksOp(i)); e != nil {
			t.Fatal(e)
		}
	}
	if e = r.Seal(f); e != nil {
		t.Fatal(e)
	}
	return f
}
func TestBlocksStrictContentAndLazyListing(t *testing.T) {
	s := Store{t.TempDir()}
	id := strings.Repeat("a", 32)
	f := captureBlocks(t, s, id, 140)
	if f.OperationCount() != 140 || len(f.blocks.frames) < 4 || len(f.Operations) != 0 {
		t.Fatal("stream was dropped or materialized")
	}
	st, err := os.Stat(f.blocks.path)
	if err != nil {
		t.Fatal(err)
	}
	privateSize := st.Size()
	if other, err := s.BeginTemporary(testFilm(id)); err == nil {
		other.Close()
		t.Fatal("new recorder reused sealed identity")
	}
	f.End.Score = 17
	if e := s.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 || rows[0].Score != 17 {
		t.Fatal(rows, e)
	}
	g, e := s.LoadEntry(rows[0])
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range []int{139, 0, 48, 139} {
		o, e := g.OperationAt(i)
		if e != nil || !bytes.Equal(o.Payload, blocksOp(i).Payload) {
			t.Fatal(i, e)
		}
	}
	raw, _ := s.Read(id)
	logical, e := Unmarshal(raw)
	if e != nil || len(logical.Operations) != 140 {
		t.Fatal(e)
	}
	for i, o := range logical.Operations {
		if !bytes.Equal(o.Payload, blocksOp(i).Payload) {
			t.Fatal(i)
		}
	}
	// Corrupt only the final compressed body. Listing and direct launch access
	// still work. The first attempt to use this body must stop.
	part := f.blocks.frames[len(f.blocks.frames)-1]
	bad := bytes.Clone(raw)
	bad[part.offset+descriptorSize] ^= 1
	path, _ := s.resolved(id, true)
	if e = os.WriteFile(path, bad, 0600); e != nil {
		t.Fatal(e)
	}
	rows, e = s.List()
	if e != nil || len(rows) != 1 || rows[0].Error != "" {
		t.Fatal(rows, e)
	}
	g, e = s.LoadEntry(rows[0])
	if e != nil || !bytes.Equal(g.Launch, f.Launch) {
		t.Fatal(e)
	}
	if _, e = g.OperationAt(139); e == nil {
		t.Fatal("used corrupt operation frame")
	}
	_, ops, n, _ := InspectPrefix(path)
	if ops != part.first || n != part.offset {
		t.Fatal(ops, n, part)
	}
	for _, cut := range []int{0, 8, 13, int(part.offset + descriptorSize + 1), len(raw) - 1, int(privateSize)} {
		if _, e := Unmarshal(raw[:cut]); e == nil {
			t.Fatal("accepted cut", cut)
		}
	}
	// A forged, correctly rehashed index must not bypass decoded-size limits.
	var forged bytes.Buffer
	m := f.blocks.manifest
	forged.Write(blockPrefix)
	forged.Write(raw[12:m.IndexOffset])
	idx := bytes.Clone(raw[m.IndexOffset : m.IndexOffset+int64(descriptorSize*m.Blocks)])
	order.PutUint32(idx[8:12], 0xffffffff)
	m.IndexDigest = BytesDigest(idx)
	forged.Write(idx)
	if e = writeManifest(&forged, m); e != nil {
		t.Fatal(e)
	}
	if _, e = Unmarshal(forged.Bytes()); e == nil {
		t.Fatal("unbounded expansion admitted")
	}
}
func TestRetentionSidecarsAndUnscored(t *testing.T) {
	s := Store{t.TempDir()}
	id := strings.Repeat("b", 32)
	f := captureBlocks(t, s, id, 0)
	if e := s.PublishLast(f); e != nil {
		t.Fatal(e)
	}
	rows, _ := s.List()
	raw, _ := s.Read(id)
	if e := s.RenameEntry(rows[0], "Last Mission"); e != nil {
		t.Fatal(e)
	}
	if e := s.RenameEntry(rows[0], "Named flight"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.LoadEntry(rows[0]); e == nil {
		t.Fatal("stale selection")
	}
	fresh := Store{s.Root}
	rows, _ = fresh.List()
	if len(rows) != 1 || rows[0].Title != "Named flight" {
		t.Fatal(rows)
	}
	after, _ := s.Read(id)
	if len(after) != len(raw) || !bytes.Equal(raw[:len(raw)-titleSlotSize], after[:len(after)-titleSlotSize]) {
		t.Fatal("rename rewrote recording")
	}
	p, _ := s.resolved(id, true)
	if _, e := os.Stat(p + ".title.json"); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	rows, _ = s.List()
	if len(rows) != 1 || rows[0].Title != "Named flight" {
		t.Fatal(rows)
	}
	u := captureBlocks(t, s, strings.Repeat("c", 32), 2)
	if e := s.PublishUnscored(u, "fixture rejection"); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.List()
	if len(rows) != 2 || !rows[1].Unscored {
		t.Fatal(rows)
	}
}
func TestKilledRecorderPrefix(t *testing.T) {
	const id = "dddddddddddddddddddddddddddddddd"
	if root := os.Getenv("T1_OWNED_PREFIX"); root != "" {
		f := testFilm(id)
		r, e := (Store{root}).BeginTemporary(f)
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 140; i++ {
			if e = r.Append(blocksOp(i)); e != nil {
				t.Fatal(e)
			}
		}
		os.Exit(39)
	}
	root := t.TempDir()
	s := Store{root}
	prior := captureBlocks(t, s, strings.Repeat("e", 32), 0)
	if e := s.PublishLast(prior); e != nil {
		t.Fatal(e)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestKilledRecorderPrefix$")
	child.Env = append(os.Environ(), "T1_OWNED_PREFIX="+root)
	e := child.Run()
	if x, ok := e.(*exec.ExitError); !ok || x.ExitCode() != 39 {
		t.Fatal(e)
	}
	blocks, ops, _, _ := InspectPrefix(filepath.Join(root, ".film-"+id+".tmp"))
	if blocks < 2 || ops < 1 || ops >= 140 {
		t.Fatal(blocks, ops)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 || rows[0].ID != prior.Header.ID {
		t.Fatal(rows, e)
	}
	next := captureBlocks(t, s, strings.Repeat("f", 32), 3)
	if e = s.PublishLast(next); e != nil {
		t.Fatal(e)
	}
}

func TestCompressedListingTracksReplacements(t *testing.T) {
	s := Store{t.TempDir()}
	id := strings.Repeat("1", 32)
	f := testFilm(id)
	f.Header.Title = "A"
	p, _ := s.path(id)
	raw, _ := Marshal(f)
	if e := os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	first, e := s.List()
	if e != nil || len(first) != 1 {
		t.Fatal(first, e)
	}
	st, _ := os.Stat(p)
	f.Header.Title = "B"
	raw, _ = Marshal(f)
	tmp := p + ".new"
	if e = os.WriteFile(tmp, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Chtimes(tmp, st.ModTime(), st.ModTime()); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(tmp, p); e != nil {
		t.Fatal(e)
	}
	next, e := s.List()
	if e != nil || len(next) != 1 || next[0].Title != "B" {
		t.Fatal(next, e)
	}
	if _, e = s.LoadEntry(first[0]); e == nil {
		t.Fatal("stale cached selection opened")
	}
	if e = os.Remove(p); e != nil {
		t.Fatal(e)
	}
	next, e = s.List()
	if e != nil || len(next) != 0 {
		t.Fatal(next, e)
	}
}
func TestCorruptSealCannotReplacePriorLast(t *testing.T) {
	s := Store{t.TempDir()}
	previous := captureBlocks(t, s, strings.Repeat("1", 32), 2)
	if e := s.PublishLast(previous); e != nil {
		t.Fatal(e)
	}
	next := captureBlocks(t, s, strings.Repeat("2", 32), 90)
	file, e := os.OpenFile(next.blocks.path, os.O_RDWR, 0)
	if e != nil {
		t.Fatal(e)
	}
	off := next.blocks.frames[1].offset + descriptorSize
	var b [1]byte
	if _, e = file.ReadAt(b[:], off); e != nil {
		t.Fatal(e)
	}
	b[0] ^= 1
	if _, e = file.WriteAt(b[:], off); e != nil {
		t.Fatal(e)
	}
	file.Close()
	if e = s.PublishLast(next); e == nil {
		t.Fatal("corrupt content became Last Mission")
	}
	last, e := s.lastID()
	if e != nil || last != previous.Header.ID {
		t.Fatal(last, e)
	}
}
