package replay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func expectName(t *testing.T, s Store, id, title string) {
	t.Helper()
	rows, e := s.List()
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if r.ID == id && r.Title == title && r.Error == "" {
			p, e := s.resolved(id, true)
			if e != nil || filepath.Base(p) != title+".g2r" {
				t.Fatal(p, e)
			}
			return
		}
	}
	t.Fatalf("expected %s %q, got %+v", id, title, rows)
}
func TestReadableFilmPublishRenameShare(t *testing.T) {
	s := Store{t.TempDir()}
	id := strings.Repeat("a", 32)
	f := captureBlocks(t, s, id, 140)
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(s.PublishLast(f))
	expectName(t, s, id, "Last Mission")
	playing, e := s.LoadEntry(Entry{ID: id})
	must(e)
	before, e := s.Read(id)
	must(e)
	must(s.Rename(id, "EUR City Patrol"))
	expectName(t, s, id, "EUR City Patrol")
	op, e := playing.OperationAt(139)
	must(e)
	if !bytes.Equal(op.Payload, blocksOp(139).Payload) {
		t.Fatal("renamed active playback")
	}
	must(s.Rename(id, "eur city patrol"))
	expectName(t, s, id, "eur city patrol")
	after, e := s.Read(id)
	must(e)
	if !bytes.Equal(before[:len(before)-2*titleSlotSize], after[:len(after)-2*titleSlotSize]) {
		t.Fatal("immutable data changed")
	}
	must(s.Rename(id, "eur city patrol"))
	same, e := s.Read(id)
	must(e)
	if !bytes.Equal(same, after) {
		t.Fatal("unchanged rename modified bytes")
	}
	shared := Store{t.TempDir()}
	must(os.WriteFile(filepath.Join(shared.Root, "External Label.G2R"), after, 0600))
	must(shared.ReconcileNames())
	must(shared.ReconcileNames())
	expectName(t, Store{shared.Root}, id, "eur city patrol")
	identical, e := shared.Read(id)
	must(e)
	if !bytes.Equal(identical, after) {
		t.Fatal("external basename normalization wrote film")
	}
	g, e := shared.Load(id)
	must(e)
	op, e = g.OperationAt(139)
	must(e)
	if !bytes.Equal(op.Payload, blocksOp(139).Payload) {
		t.Fatal("shared operations changed")
	}
	must(shared.Delete(id))
	if _, e = shared.Load(id); !os.IsNotExist(e) {
		t.Fatal("delete failed", e)
	}
}
func TestReadableFilmCollisionsAndRetention(t *testing.T) {
	s := Store{t.TempDir()}
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	publish := func(c string, unscored bool) string {
		id := strings.Repeat(c, 32)
		f := testFilm(id)
		if unscored {
			must(s.PublishUnscored(f, "fixture disposition"))
		} else {
			must(s.PublishLast(f))
		}
		return id
	}
	a := publish("a", false)
	must(s.Rename(a, "First 12345678901234"))
	b := publish("b", false)
	must(s.Rename(b, "FIRST 12345678901234"))
	expectName(t, s, b, "FIRST 1234567890 (2)")
	must(s.Rename(a, "../A?B"))
	expectName(t, s, a, "_A_B")
	must(s.Rename(a, "CON.txt"))
	expectName(t, s, a, "_CON.txt")
	blocker := filepath.Join(s.Root, "Last Mission.g2r")
	must(os.WriteFile(blocker, []byte("unrelated"), 0600))
	c := publish("c", false)
	expectName(t, s, c, "Last Mission (2)")
	d := publish("d", true)
	expectName(t, s, d, "Unscored Mission")
	e := publish("e", true)
	expectName(t, s, e, "Unscored Mission (2)")
	last, err := s.lastID()
	must(err)
	if last != c {
		t.Fatal(last)
	}
	next := publish("f", false)
	expectName(t, s, next, "Last Mission (2)")
	if _, err = s.Load(c); !os.IsNotExist(err) {
		t.Fatal("old rolling film", err)
	}
	for _, id := range []string{d, e} {
		film, err := s.Load(id)
		must(err)
		_, err = s.Unscored(film)
		must(err)
	}
	raw, err := os.ReadFile(blocker)
	must(err)
	if string(raw) != "unrelated" {
		t.Fatal("overwrote unrelated destination")
	}
}
func TestReadableFilmInterruptedNameEdit(t *testing.T) {
	for cut := 0; cut < 4; cut++ {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			s := Store{t.TempDir()}
			id := strings.Repeat("c", 32)
			must := func(e error) {
				t.Helper()
				if e != nil {
					t.Fatal(e)
				}
			}
			must(s.PublishLast(testFilm(id)))
			p, e := s.resolved(id, true)
			must(e)
			m, e := metadataAt(p)
			must(e)
			x := nameEdit{ID: id, Binding: fmt.Sprintf("%x", m.portable.binding), From: filepath.Base(p), Previous: m.Header.Title, Title: "After restart", Generation: m.portable.generation, ReleaseLast: true}
			data, e := json.Marshal(x)
			must(e)
			must(writeNameIntent(filepath.Join(s.Root, editJournal), data))
			dest := filepath.Join(s.Root, x.Title+".g2r")
			if cut == 3 {
				must(os.WriteFile(dest, []byte("occupied"), 0600))
			} else if cut > 0 {
				must(moveFilmNew(p, dest))
				if cut == 2 {
					must(renameSlot(dest, m.portable, x.Title))
				}
			}
			fresh := Store{s.Root}
			if cut == 3 {
				if err := fresh.ReconcileNames(); err == nil {
					t.Fatal("unreadable unrelated file not reported")
				}
				expectName(t, s, id, "Last Mission")
				raw, e := os.ReadFile(dest)
				must(e)
				if string(raw) != "occupied" {
					t.Fatal("collision")
				}
			} else {
				must(fresh.ReconcileNames())
				expectName(t, fresh, id, x.Title)
				last, e := fresh.lastID()
				must(e)
				if last != "" {
					t.Fatal("retention incomplete")
				}
			}
			if _, e := os.Stat(filepath.Join(s.Root, editJournal)); !os.IsNotExist(e) {
				t.Fatal(e)
			}
		})
	}
}
func TestReadableFilmWindowsOpenHandle(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows delete-sharing semantics")
	}
	s := Store{t.TempDir()}
	id := strings.Repeat("d", 32)
	if e := s.PublishLast(testFilm(id)); e != nil {
		t.Fatal(e)
	}
	p, e := s.resolved(id, true)
	if e != nil {
		t.Fatal(e)
	}
	file, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	err := s.Rename(id, "New name")
	if e = file.Close(); e != nil {
		t.Fatal(e)
	}
	if err == nil {
		t.Fatal("renamed despite non-delete-sharing handle")
	}
	expectName(t, s, id, "Last Mission")
	if last, e := s.lastID(); e != nil || last != id {
		t.Fatal(last, e)
	}
	if e = s.Rename(id, "New name"); e != nil {
		t.Fatal(e)
	}
	expectName(t, s, id, "New name")
}

func TestReadableFilmReadOnlyTitleFailure(t *testing.T) {
	s := Store{t.TempDir()}
	id := strings.Repeat("b", 32)
	if err := s.PublishLast(testFilm(id)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Root, "Last Mission.g2r")
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) })
	if f, err := os.OpenFile(path, os.O_RDWR, 0); err == nil {
		f.Close()
		t.Skip("effective account can write read-only files")
	}
	if err := s.Rename(id, "Attempt"); err == nil {
		t.Fatal("title write unexpectedly succeeded")
	}
	expectName(t, s, id, "Last Mission")
	if _, e := os.Stat(filepath.Join(s.Root, editJournal)); !os.IsNotExist(e) {
		t.Fatal("failed rename blocks other publications", e)
	}
	if last, e := s.lastID(); e != nil || last != id {
		t.Fatal(last, e)
	}
}
