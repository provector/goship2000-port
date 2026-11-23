package filelease

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetireOnlyProvenIdleLegacyReservations(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "assets.prepare.lock")
	internal := filepath.Join(dir, ".runtime-locks")
	if e := os.Mkdir(internal, 0700); e != nil {
		t.Fatal(e)
	}
	anchor := filepath.Join(internal, "assets.prepare.lock.lease")
	if e := os.WriteFile(anchor, nil, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Link(anchor, marker); e != nil {
		t.Fatal(e)
	}
	f, e := openExisting(anchor)
	if e != nil {
		t.Fatal(e)
	}
	if e = tryLock(f); e != nil {
		f.Close()
		t.Fatal(e)
	}
	e = Retire([]string{marker}, nil)
	f.Close()
	if e == nil {
		t.Fatal("removed live lease")
	}
	if _, e = os.Stat(marker); e != nil {
		t.Fatal(e)
	}
	other := filepath.Join(dir, "unknown.lock")
	if e = os.WriteFile(other, []byte("retain"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = Retire([]string{marker}, nil); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{marker, anchor, internal} {
		if _, e = os.Lstat(p); !os.IsNotExist(e) {
			t.Fatal(p, e)
		}
	}
	if b, e := os.ReadFile(other); e != nil || string(b) != "retain" {
		t.Fatal("changed unknown reservation")
	}
}
