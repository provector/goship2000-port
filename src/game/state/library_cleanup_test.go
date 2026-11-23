package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompletedLibraryCleanup(t *testing.T) {
	app := t.TempDir()
	root := filepath.Join(app, "saves")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	if e := relocateLibraries(app, root); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(app, libraryBackup)
	if _, e := os.Stat(backup); !os.IsNotExist(e) {
		t.Fatal("fresh startup created backup", e)
	}
	// Separate old installation, with two independently owned documents.
	app = t.TempDir()
	root = filepath.Join(app, "saves")
	backup = filepath.Join(app, libraryBackup)
	for _, d := range []string{root, filepath.Join(app, "missions")} {
		if e := os.Mkdir(d, 0700); e != nil {
			t.Fatal(e)
		}
	}
	for _, n := range []string{"keep.g2m", "dupe.g2m"} {
		if e := os.WriteFile(filepath.Join(app, "missions", n), []byte(n), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := migrateLibraries(app, root); e != nil {
		t.Fatal(e)
	} // Stop immediately after commit, before cleanup.
	dst := filepath.Join(root, "missions", "keep.g2m")
	if e := os.WriteFile(dst, []byte("updated"), 0600); e != nil {
		t.Fatal(e)
	}
	unknown := filepath.Join(backup, "missions", "unidentified")
	if e := os.WriteFile(unknown, []byte("recovery"), 0600); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e := relocateLibraries(app, root); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := os.Stat(filepath.Join(backup, "missions", "dupe.g2m")); !os.IsNotExist(e) {
		t.Fatal("verified duplicate survived", e)
	}
	for path, want := range map[string]string{dst: "updated", filepath.Join(backup, "missions", "keep.g2m"): "keep.g2m", unknown: "recovery", filepath.Join(root, "missions", "dupe.g2m"): "dupe.g2m"} {
		b, e := os.ReadFile(path)
		if e != nil || string(b) != want {
			t.Fatal(path, e)
		}
	}
}
