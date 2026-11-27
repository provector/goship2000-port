package playerdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndependentArchiveFallbackAndScratch(t *testing.T) {
	root := t.TempDir()
	s := filepath.Join(root, "保存")
	old := filepath.Join(s, "imports")
	if e := os.MkdirAll(old, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(old, "provenance.json"), []byte("history"), 0600); e != nil {
		t.Fatal(e)
	}
	dst := filepath.Join(root, "imports")
	if e := os.Mkdir(dst, 0700); e != nil {
		t.Fatal(e)
	}
	if got := Archive(s, "imports", false); got != old {
		t.Fatal("claimed occupied destination", got)
	}
	if e := os.Remove(dst); e != nil {
		t.Fatal(e)
	}
	if got := Archive(s, "imports", false); got != dst {
		t.Fatal(got)
	}
	for i := 0; i < 2; i++ {
		if !OwnedArchive(s, "imports") || Archive(s, "imports", false) != dst {
			t.Fatal("lost archive")
		}
	}
	if got, e := os.ReadFile(filepath.Join(dst, "provenance.json")); e != nil || string(got) != "history" {
		t.Fatal(e)
	}
	other := filepath.Join(root, "other")
	if Archive(other, "imports", true) != filepath.Join(other, "imports") {
		t.Fatal("cross-store access")
	}
	scratch, e := Scratch(s)
	if e != nil || filepath.Dir(scratch) != Sibling(s, "temp") {
		t.Fatal(scratch, e)
	}
	scratch2, e := Scratch(s)
	if e != nil || scratch == scratch2 {
		t.Fatal(scratch2, e)
	}
	if _, e = os.Stat(scratch); e != nil {
		t.Fatal("removed active work", e)
	}
	s = filepath.Join(t.TempDir(), "saves")
	if e = os.WriteFile(Sibling(s, "temp"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	scratch, e = Scratch(s)
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(scratch)
	if filepath.Dir(scratch) != os.TempDir() {
		t.Fatal("OS fallback", scratch)
	}
	bad := filepath.Join(t.TempDir(), "missing")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, bad)
	}
	if _, e = Scratch(s); e == nil {
		t.Fatal("claimed unavailable scratch")
	}
}

func TestReservedArchiveNameRemainsAUsableCareerDirectory(t *testing.T) {
	for _, name := range []string{"reports", "imports"} {
		dir := filepath.Join(t.TempDir(), name)
		if err := Directory(dir); err != nil {
			t.Fatal(err)
		}
		if got := Archive(dir, name, true); got != filepath.Join(dir, name) || OwnedArchive(dir, name) {
			t.Fatal("career directory claimed as archive", got)
		}
	}
}
