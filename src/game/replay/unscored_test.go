package replay

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnscoredPublicationPreservesFilmPriorLastAndIdentity(t *testing.T) {
	s := Store{Root: t.TempDir()}
	prior := testFilm(strings.Repeat("1", 32))
	if e := s.PublishLast(prior); e != nil {
		t.Fatal(e)
	}
	index, _ := os.ReadFile(filepath.Join(s.Root, "last.json"))
	previous, _ := s.Read(prior.Header.ID)
	f := testFilm(strings.Repeat("2", 32))
	raw, e := Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = s.PublishUnscored(f, "unknown nonplayer result"); e != nil {
			t.Fatal(e)
		}
	}
	got, e := s.Read(f.Header.ID)
	_, _, _, portable, err := readContainer(bytes.NewReader(got), int64(len(got)))
	if e != nil || err != nil || portable == nil || !bytes.Equal(raw[12:], got[12:portable.core]) {
		t.Fatal("sealed body changed", e)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "last.json"))
	old, _ := s.Read(prior.Header.ID)
	if !bytes.Equal(index, after) || !bytes.Equal(previous, old) {
		t.Fatal("previous film/index overwritten")
	}
	rows, e := (Store{Root: s.Root}).List()
	if e != nil || len(rows) != 2 {
		t.Fatal(rows, e)
	}
	found := false
	for _, r := range rows {
		if r.ID == f.Header.ID {
			found = r.Unscored && r.Title == "Unscored Mission" && r.Complete
		}
	}
	if !found {
		t.Fatal("unscored film hidden behindLast", rows)
	}
	bad := *f
	bad.End.Score++
	if e = s.PublishUnscored(&bad, "later"); e == nil {
		t.Fatal("different content reused identity")
	}
	if e = s.Rename(f.Header.ID, "Kept evidence"); e != nil {
		t.Fatal(e)
	}
	f2, e := s.Load(f.Header.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Unscored(f2); e != nil {
		t.Fatal("explicit title rename lostunscored identity", e)
	}
	if e = s.Delete(f.Header.ID); e != nil {
		t.Fatal(e)
	}
	p, _ := s.unscoredPath(f.Header.ID)
	if _, e = os.Stat(p); !os.IsNotExist(e) {
		t.Fatal("delete left disposition", e)
	}
}
func TestUnscoredWriteFailureAndIncompleteStayEvidence(t *testing.T) {
	s := Store{Root: t.TempDir()}
	f := testFilm(strings.Repeat("3", 32))
	f.Header.Complete = false
	if e := s.PublishUnscored(f, "rejected"); e == nil {
		t.Fatal("incomplete published")
	}
	f.Header.Complete = true
	if e := os.WriteFile(filepath.Join(s.Root, editJournal), []byte("owned guard"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := s.PublishUnscored(f, "rejected"); e == nil {
		t.Fatal("invalid rename journal ignored")
	}
	if _, e := s.Load(f.Header.ID); !os.IsNotExist(e) {
		t.Fatal("failed publish created playable film", e)
	}
	os.Remove(filepath.Join(s.Root, editJournal))
	if e := s.PublishUnscored(f, "rejected"); e != nil {
		t.Fatal("retry", e)
	}
	g := testFilm(strings.Repeat("4", 32))
	path, _ := s.unscoredPath(g.Header.ID)
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e := s.PublishUnscored(g, "rejected"); e == nil {
		t.Fatal("invalid status owner overwritten")
	}
	prefix := ".film-" + g.Header.ID + ".tmp"
	os.WriteFile(filepath.Join(s.Root, prefix), []byte("retainedprefix"), 0600)
	os.WriteFile(filepath.Join(s.Root, prefix+".failure.json"), []byte(`{"Version":1,"FilmID":"`+g.Header.ID+`","Phase":"append","Cause":"capacity reached","Operation":5,"Step":4,"Bytes":100,"Limit":100}`), 0600)
	notices, e := s.RecordingNotices()
	if e != nil || len(notices) != 1 || !strings.Contains(notices[0], "not playable") || !strings.Contains(notices[0], "capacity reached") {
		t.Fatal(notices, e)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 {
		t.Fatal("prefix appeared as complete row", rows, e)
	}
}
