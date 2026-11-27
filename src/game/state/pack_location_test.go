package state

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/internal/userpack"
)

// Construct a real pre-relocation store, including shared metadata, then use
// the ordinary open/commit paths for the continuation and the next restart.
func legacyLayout(t *testing.T, app string) (string, SaveState) {
	t.Helper()
	root := filepath.Join(app, "São Łódź", "careers")
	s, e := OpenManagedCareer(root, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	pendingTestPilot(t, s)
	want := s.State()
	s.Close()
	for i := 0; i < 6; i++ {
		name := playerPack(i)
		if e = os.Rename(filepath.Join(root, "pilots", name), filepath.Join(root, name)); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.Remove(filepath.Join(root, packLocation)); e != nil {
		t.Fatal(e)
	}
	return root, want
}

func TestCareerRelocationInterruptedPublication(t *testing.T) {
	if app := os.Getenv("GOSHIP_LAYOUT_CUT"); app != "" {
		root := filepath.Join(app, "São Łódź", "careers")
		old := &careerPacks{dir: root, root: root, cfg: filepath.Join(app, userpack.ConfigName)}
		if e := old.refresh(); e != nil {
			t.Fatal(e)
		}
		dst := filepath.Join(root, "pilots")
		loc, e := planPackCopies(old, dst)
		if e != nil {
			t.Fatal(e)
		}
		limit := 6
		if os.Getenv("GOSHIP_CUT_PHASE") == "partial" {
			limit = 3
		}
		for i := 0; i < limit; i++ {
			n := playerPack(i)
			if e = copyChecked(filepath.Join(root, n), filepath.Join(dst, n), loc.Original[i]); e != nil {
				t.Fatal(e)
			}
		}
		if os.Getenv("GOSHIP_CUT_PHASE") == "receipt" {
			raw, _ := json.Marshal(loc)
			if e = writeSaveAtomic(root, packLocation, raw); e != nil {
				t.Fatal(e)
			}
			if e = os.Remove(filepath.Join(root, playerPack(0))); e != nil {
				t.Fatal(e)
			}
		}
		os.Exit(39)
	}
	for _, phase := range []string{"partial", "copied", "receipt"} {
		t.Run(phase, func(t *testing.T) {
			app := t.TempDir()
			root, want := legacyLayout(t, app)
			cfg := filepath.Join(app, userpack.ConfigName)
			before, _ := os.ReadFile(cfg)
			var hashes [6]string
			for i := range hashes {
				var e error
				hashes[i], e = userpack.HashFile(filepath.Join(root, playerPack(i)))
				if e != nil {
					t.Fatal(e)
				}
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestCareerRelocationInterruptedPublication$")
			cmd.Env = append(os.Environ(), "GOSHIP_LAYOUT_CUT="+app, "GOSHIP_CUT_PHASE="+phase)
			out, e := cmd.CombinedOutput()
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 39 {
				t.Fatal(e, string(out))
			}
			s, e := OpenManagedCareer(root, "", app, true)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			after, e := os.ReadFile(cfg)
			if e != nil || !bytes.Equal(before, after) || !reflect.DeepEqual(s.State(), want) {
				t.Fatal("career or shared configuration changed", e)
			}
			for i, w := range hashes {
				n := playerPack(i)
				h, e := userpack.HashFile(filepath.Join(root, "pilots", n))
				if e != nil || h != w {
					t.Fatal(n, e)
				}
				if _, e = os.Lstat(filepath.Join(root, n)); !os.IsNotExist(e) {
					t.Fatal("unretired", n, e)
				}
			}
			old0, e := os.ReadFile(filepath.Join(root, "pilots", playerPack(0)))
			if e != nil {
				t.Fatal(e)
			}
			revised := s.State()
			revised.SelectedPilot().Name = "After migration"
			if e = s.Commit(revised); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(root, playerPack(0)), old0, 0600); e != nil {
				t.Fatal(e)
			}
			s.Close()
			again, e := OpenManagedCareer(root, "", app, true)
			if e != nil {
				t.Fatal(e)
			}
			if again.State().SelectedPilot().Name != "After migration" {
				t.Fatal("stale root replaced progress")
			}
			again.Close()
			if e = os.Remove(filepath.Join(root, "pilots", playerPack(0))); e != nil {
				t.Fatal(e)
			}
			if _, e = OpenManagedCareer(root, "", app, true); e == nil {
				t.Fatal("missing committed pack caused fallback")
			}
		})
	}
}
func TestCareerRelocationKeepsUsableOriginalOnCollision(t *testing.T) {
	for _, obstruction := range []string{"pilots", "slot", "receipt"} {
		t.Run(obstruction, func(t *testing.T) {
			app := t.TempDir()
			root, want := legacyLayout(t, app)
			dst := filepath.Join(root, "pilots")
			switch obstruction {
			case "pilots":
				if e := os.Remove(dst); e != nil {
					t.Fatal(e)
				}
				dst = filepath.Join(root, "pilots")
			case "slot":
				dst = filepath.Join(dst, playerPack(3))
			case "receipt":
				dst = filepath.Join(root, pendingLocation)
			}
			if e := os.WriteFile(dst, []byte("unrelated"), 0600); e != nil {
				t.Fatal(e)
			}
			s, e := OpenManagedCareer(root, "", app, true)
			if e != nil {
				t.Fatal(e)
			}
			if s.PackDirectory() != root || !reflect.DeepEqual(want, s.State()) {
				t.Fatal("lost original")
			}
			changed := s.State()
			changed.SelectedPilot().Name = "During deferral"
			if e = s.Commit(changed); e != nil {
				t.Fatal(e)
			}
			s.Close()
			raw, e := os.ReadFile(dst)
			if e != nil || string(raw) != "unrelated" {
				t.Fatal("overwrote occupied destination", e)
			}
			if e = os.Remove(dst); e != nil {
				t.Fatal(e)
			}
			s, e = OpenManagedCareer(root, "", app, true)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			if s.State().SelectedPilot().Name != "During deferral" || s.PackDirectory() != filepath.Join(root, "pilots") {
				t.Fatal("failed to retry with new progress")
			}
		})
	}
}

func TestUnfinishedRelocationCannotInitializeDefaults(t *testing.T) {
	app := t.TempDir()
	root, _ := legacyLayout(t, app)
	if err := os.WriteFile(filepath.Join(root, pendingLocation), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if err := os.Rename(filepath.Join(root, playerPack(i)), filepath.Join(app, playerPack(i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := OpenManagedCareer(root, "", app, true); err == nil {
		t.Fatal("initialized over an incomplete migration")
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(root, "pilots", playerPack(i))); !os.IsNotExist(err) {
			t.Fatal("unexpected new pack", err)
		}
	}
}

func TestDeferredCopyCanCatchUpWithOldWriter(t *testing.T) {
	app := t.TempDir()
	root, _ := legacyLayout(t, app)
	p := &careerPacks{dir: root, root: root, cfg: filepath.Join(app, userpack.ConfigName)}
	if e := p.refresh(); e != nil {
		t.Fatal(e)
	}
	dst := filepath.Join(root, "pilots")
	plan, e := planPackCopies(p, dst)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		n := playerPack(i)
		if e = copyChecked(filepath.Join(root, n), filepath.Join(dst, n), plan.Original[i]); e != nil {
			t.Fatal(e)
		}
	}
	// Previous releases can still commit between an uncommitted copy and retry.
	old := SaveStore{Directory: root, packed: p}
	if e = old.load(); e != nil {
		t.Fatal(e)
	}
	changed := old.State()
	changed.SelectedPilot().Name = "Updated on old writer"
	if e = old.Commit(changed); e != nil {
		t.Fatal(e)
	}
	s, e := OpenManagedCareer(root, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.PackDirectory() != dst || s.State().SelectedPilot().Name != "Updated on old writer" {
		t.Fatal("resumed copy lost the intervening commit")
	}
}

func TestCareerRecoveryEvidencePreventsFreshInitialization(t *testing.T) {
	for _, name := range []string{careerBackup, "sorties"} {
		t.Run(name, func(t *testing.T) {
			app := t.TempDir()
			root := filepath.Join(app, "saves")
			if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, name, "retained"), []byte("prior career"), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := OpenManagedCareer(root, "", app, true)
			if err == nil {
				s.Close()
				t.Fatal("initialized a replacement over recovery evidence")
			}
			if existingPacks(root) || existingPacks(filepath.Join(root, "pilots")) {
				t.Fatal("published replacement pilots")
			}
		})
	}
}
