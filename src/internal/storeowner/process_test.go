package storeowner

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A fresh process must observe the existing owner even with a different TMPDIR.
func TestOverlappingTreesAcrossProcesses(t *testing.T) {
	if path := os.Getenv("GOSHIP_TEST_OWNER_CHILD"); path != "" {
		f, err := Acquire(Tree{Path: path, Write: true})
		if os.Getenv("GOSHIP_TEST_OWNER_BUSY") == "yes" {
			if f != nil {
				f.Close()
			}
			if !errors.Is(err, ErrInUse) {
				t.Fatalf("wanted an existing owner, got %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		} else if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(root, "saved", "pilots")
	check := func(path, busy string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestOverlappingTreesAcrossProcesses$")
		cmd.Env = append(os.Environ(), "GOSHIP_TEST_OWNER_CHILD="+path, "GOSHIP_TEST_OWNER_BUSY="+busy, "TMPDIR="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child %s: %v\n%s", path, err, out)
		}
	}
	f, err := Acquire(Tree{Path: owned, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = os.MkdirAll(owned, 0700); err != nil {
		t.Fatal(err)
	}
	check(owned, "yes")
	check(filepath.Dir(owned), "yes")
	check(filepath.Join(owned, "slot"), "yes")
	check(filepath.Join(root, "independent"), "no")
	check(owned, "yes")
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	check(owned, "no")
}
