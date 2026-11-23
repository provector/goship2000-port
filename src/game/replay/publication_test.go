package replay

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicationConflictsPreservePriorIndexAndFilm(t *testing.T) {
	s := Store{Root: t.TempDir()}
	prior := testFilm(strings.Repeat("1", 32))
	next := testFilm(strings.Repeat("2", 32))
	if e := s.PublishLast(prior); e != nil {
		t.Fatal(e)
	}
	old, _ := s.Read(prior.Header.ID)
	index, _ := os.ReadFile(filepath.Join(s.Root, "last.json"))
	lock := filepath.Join(s.Root, editJournal)
	os.WriteFile(lock, []byte("other owned writer"), 0600)
	if e := s.PublishLast(next); e == nil {
		t.Fatal("invalid rename journal admitted")
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "last.json"))
	if !bytes.Equal(index, after) {
		t.Fatal("index changed")
	}
	os.Remove(lock)
	os.WriteFile(filepath.Join(s.Root, "last.json"), []byte("invalid but preserved"), 0600)
	if e := s.PublishLast(next); e == nil {
		t.Fatal("invalid predecessor overwritten")
	}
	after, _ = os.ReadFile(filepath.Join(s.Root, "last.json"))
	if string(after) != "invalid but preserved" {
		t.Fatal("invalid index replaced")
	}
	got, _ := s.Read(prior.Header.ID)
	if !bytes.Equal(old, got) {
		t.Fatal("prior film changed")
	}
}
func TestPostRenameIndexRestorePreservesExactBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "last.json")
	prior := []byte("{\n  \"Version\":1,\"ID\":\"11111111111111111111111111111111\"\n}\n")
	os.WriteFile(path, []byte("new complete pointer"), 0600)
	if e := restoreFilmIndex(path, prior, true); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(prior, after) {
		t.Fatal("predecessor not restored exactly")
	}
	if e := restoreFilmIndex(path, nil, false); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("new pointer remains")
	}
}
