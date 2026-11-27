package legacyreplay

import (
	"bytes"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"testing"
)

func TestManualConversionAndMixedLibrary(t *testing.T) {
	path := os.Getenv("W40_RELEASED_FILM")
	if path == "" {
		t.Skip("supply a disposable released film copy")
	}
	dir := t.TempDir()
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	name := filepath.Join(dir, "test.g2r")
	write := func(path string, b []byte) {
		t.Helper()
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(name, original)
	write(filepath.Join(dir, "empty.g2r"), nil)
	write(filepath.Join(dir, "unrelated.g2r"), bytes.Repeat([]byte{'x'}, 64))
	write(filepath.Join(dir, "truncated.g2r"), original[:20])
	if rows, e := (replay.Store{Root: dir}).List(); e != nil {
		t.Fatal(e)
	} else {
		for _, r := range rows {
			if r.Complete {
				t.Fatal("legacy film admitted")
			}
		}
	}
	control := filepath.Join(t.TempDir(), "output.g2r")
	if e = Convert(name, control); e != nil {
		t.Fatal(e)
	}
	dup := filepath.Join(dir, "duplicate.g2r")
	write(dup, original)
	if s := ConvertLibrary(dir); s.Converted != 0 || s.Failed != 4 {
		t.Fatal(s)
	}
	if e = os.Remove(dup); e != nil {
		t.Fatal(e)
	}
	if s := ConvertLibrary(dir); s.Converted != 1 || s.Skipped != 1 || s.Failed != 2 {
		t.Fatal(s)
	}
	if s := ConvertLibrary(dir); s.Converted != 0 || s.Current != 1 || s.Failed != 2 {
		t.Fatal(s)
	}
	a, e := os.ReadFile(control)
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(name)
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("outputs differ", e)
	}
	b, e = os.ReadFile(filepath.Join(dir, "legacy-originals", "test.g2r"))
	if e != nil || !bytes.Equal(b, original) {
		t.Fatal("original changed", e)
	}
	rows, e := (replay.Store{Root: dir}).List()
	if e != nil {
		t.Fatal(e)
	}
	var converted replay.Entry
	good := 0
	for _, r := range rows {
		if r.Complete {
			good++
			converted = r
		}
	}
	if good != 1 {
		t.Fatal(rows)
	}
	// An externally labelled converted film remains portable. A library rename
	// followed by reopening must resolve its embedded ID to its readable basename.
	store := replay.Store{Root: dir}
	if e = store.Rename(converted.ID, "Crash Retest"); e != nil {
		t.Fatal(e)
	}
	named := filepath.Join(dir, "Crash Retest.g2r")
	if e = os.Rename(named, filepath.Join(dir, converted.ID+".g2r")); e != nil {
		t.Fatal(e)
	}
	reopened := replay.Store{Root: dir}
	if err := reopened.ReconcileNames(); err != nil {
		t.Log("mixed-directory reconciliation:", err)
	}
	if _, e = os.Stat(named); e != nil {
		t.Fatal(e)
	}
	film, e := reopened.Load(converted.ID)
	if e != nil {
		t.Fatal(e)
	}
	if film.Header.ID != converted.ID || film.OperationCount() == 0 {
		t.Fatal("converted film lost identity or operations")
	}
	if _, e = film.OperationAt(film.OperationCount() - 1); e != nil {
		t.Fatal(e)
	}

}
