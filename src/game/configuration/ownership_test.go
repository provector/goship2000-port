package configuration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnershipIndependentAndOverlappingStores(t *testing.T) {
	a, b, shared := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "new", "saves")
	for _, app := range []string{a, b} {
		d, _, e := Initialize(filepath.Join(app, Filename))
		if e != nil {
			t.Fatal(e)
		}
		d.Config.Saves = shared
		if e = d.Save(); e != nil {
			t.Fatal(e)
		}
	}
	owner, e := BeginOwnership(a, "installed", "", false)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	if e = os.MkdirAll(shared, 0700); e != nil {
		t.Fatal(e)
	}
	for _, fn := range []func() (*os.File, error){
		func() (*os.File, error) { return BeginOwnership(b, "installed", "", false) },
		func() (*os.File, error) { return OwnOutput(filepath.Join(shared, "replays", "new.g2r")) },
		func() (*os.File, error) { return OwnOutput(filepath.Dir(shared)) },
	} {
		if f, e := fn(); e == nil {
			f.Close()
			t.Fatal("overlapping owner admitted")
		} else if !strings.Contains(e.Error(), "already in use") {
			t.Fatal(e)
		}
	}
	f, e := BeginOwnership(t.TempDir(), "installed", "", false)
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	real := t.TempDir()
	link := filepath.Join(real, "alias")
	if e = os.Symlink(shared, link); e == nil {
		if f, e := OwnOutput(link); e == nil {
			f.Close()
			t.Fatal("path alias bypassed writer")
		}
	}
}
