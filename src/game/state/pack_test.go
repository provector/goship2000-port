package state

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/configuration"
	"gunship2000-assets/internal/userpack"
)

func TestPackedResultPhoenixAndSlotReplacement(t *testing.T) {
	app := t.TempDir()
	dir := filepath.Join(app, "custom")
	s, e := OpenManagedCareer(dir, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	original := pendingTestPilot(t, s)
	active, e := s.BeginSortie(pendingTestStart())
	if e != nil {
		t.Fatal(e)
	}
	proposal := pendingTestProposal(t, s, active)
	if e = s.StageSortieResult(active.ID, proposal); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = s.CommitSortie(active.ID); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenManagedCareer(dir, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.State().SelectedPilot().Statistics.CareerScore != 30 {
		t.Fatal("career score after restart")
	}
	backup, e := s.RecoveryCandidate()
	if e != nil || backup == nil || !reflect.DeepEqual(backup.State.SelectedPilot(), original) {
		t.Fatal("Phoenix backup", e)
	}
	next := s.State()
	p, e := NewPilot()
	if e != nil {
		t.Fatal(e)
	}
	p.Name = "New owner"
	next.SelectedPilotID = p.ID
	next.Slots[0] = p
	if e = s.Commit(next); e != nil {
		t.Fatal(e)
	}
	if b, e := s.RecoveryCandidate(); e != nil || b != nil {
		t.Fatal("replacement inherited Phoenix", e)
	}
	if e = s.CommitSortie(active.ID); e == nil {
		t.Fatal("replacement acknowledged prior pilot result")
	}
	for i := 0; i < 6; i++ {
		a, e := userpack.Open(filepath.Join(dir, playerPack(i)))
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := a.Get("slot.json")
		var h slotHeader
		if json.Unmarshal(raw, &h) != nil || h.Slot != i || i > 0 && h.Pilot != "" {
			t.Fatal(h)
		}
	}
}
func TestTransferAndRestart(t *testing.T) {
	app := t.TempDir()
	dir := filepath.Join(app, "saves")
	old, e := OpenSaveStore(dir, "")
	if e != nil {
		t.Fatal(e)
	}
	p := pendingTestPilot(t, old)
	v := old.State()
	q, _ := NewPilot()
	q.Name = "Missing"
	q.Status = StatusMIA
	q.LegacyStatus = 3
	q.LegacyRaw[0x56] = 3
	v.Slots[1] = q
	r, _ := NewPilot()
	r.Name = "Inactive"
	r.Status = StatusInactive
	v.Slots[2] = r
	if e = old.Commit(v); e != nil {
		t.Fatal(e)
	}
	if e = old.PrepareMissionSnapshot(); e != nil {
		t.Fatal(e)
	}
	before := old.State()
	films := filepath.Join(app, "films")
	os.Mkdir(films, 0700)
	for _, n := range []string{"example.g2r", "example.title.json", "last.json", ".spool-prefix"} {
		if e = os.WriteFile(filepath.Join(films, n), []byte(n), 0600); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 2; i++ {
		if e = MigrateCareer(dir, "", app); e != nil {
			t.Fatal(e)
		}
	}
	s, e := OpenManagedCareer(dir, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !reflect.DeepEqual(before, s.State()) || s.State().SelectedPilotID != p.ID {
		t.Fatal("roster changed")
	}
	for _, n := range []string{"example.g2r", "example.title.json", "last.json", ".spool-prefix"} {
		b, e := os.ReadFile(filepath.Join(dir, "replays", n))
		if e != nil || string(b) != n {
			t.Fatal(n, e)
		}
	}
	owner, e := configuration.BeginOwnership(app, "installed", "", false)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerChild$")
	cmd.Env = append(os.Environ(), "T3_OWNER_DIR="+app)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(string(out), e)
	}
}
func TestOwnerChild(t *testing.T) {
	app := os.Getenv("T3_OWNER_DIR")
	if app == "" {
		t.Skip("child only")
	}
	if s, e := configuration.BeginOwnership(app, "installed", "", false); e == nil {
		s.Close()
		t.Fatal("acquired another process's career")
	}
}
func TestPartialSlotJournal(t *testing.T) {
	app := os.Getenv("T3_INTERRUPT_APP")
	if app == "" {
		app = t.TempDir()
		cmd := exec.Command(os.Args[0], "-test.run=^TestPartialSlotJournal$")
		cmd.Env = append(os.Environ(), "T3_INTERRUPT_APP="+app)
		out, e := cmd.CombinedOutput()
		if exit, ok := e.(*exec.ExitError); !ok || exit.ExitCode() != 37 {
			t.Fatal("child failed before durable cut point", e, string(out))
		}
		dir := filepath.Join(app, "saves")
		raw, e := os.ReadFile(filepath.Join(dir, packIntent))
		if e != nil {
			t.Fatal(e)
		}
		var j packJournal
		if e = json.Unmarshal(raw, &j); e != nil {
			t.Fatal(e)
		}
		s, e := OpenManagedCareer(dir, "", app, true)
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		if s.State().Revision != 1 || !bytes.Equal(s.packed.meta, j.After) {
			t.Fatal("journal revision not restored")
		}
		for _, v := range j.Images {
			h, _ := userpack.HashFile(filepath.Join(dir, playerPack(v.Slot)))
			if h != v.After {
				t.Fatal("partial slot publication")
			}
		}
		return
	}
	dir := filepath.Join(app, "saves")
	s, e := OpenManagedCareer(dir, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	lease, e := configuration.BeginOwnership(app, "installed", "", false)
	if e != nil {
		t.Fatal(e)
	}
	defer lease.Close()
	p := s.packed
	var meta storeHeader
	if e = json.Unmarshal(p.meta, &meta); e != nil {
		t.Fatal(e)
	}
	meta.Revision++
	after, _ := json.Marshal(meta)
	j := packJournal{Version: 1, Store: p.id, Before: p.meta, After: after}
	for i := 0; i < 2; i++ {
		a := p.slots[i]
		h, _ := a.Get("slot.json")
		a["slot.json"] = userpack.Value(append(h, '\n'))
		stage, e := userpack.Stage(dir, a)
		if e != nil {
			t.Fatal(e)
		}
		old, _ := userpack.HashFile(filepath.Join(dir, playerPack(i)))
		newHash, _ := userpack.HashFile(stage)
		j.Images = append(j.Images, packImage{i, filepath.Base(stage), old, newHash})
	}
	b, _ := json.Marshal(j)
	if e = writeSaveAtomic(dir, packIntent, b); e != nil {
		t.Fatal(e)
	}
	if e = userpack.Replace(filepath.Join(dir, j.Images[0].Stage), filepath.Join(dir, playerPack(0))); e != nil {
		t.Fatal(e)
	}
	os.Exit(37) // Simulate process death while both kernel leases are held.
}

func TestImportedRosterReopensFromPacks(t *testing.T) {
	app := t.TempDir()
	dir := filepath.Join(app, "saves")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	raw := syntheticLegacyRoster()
	before, e := ParseLegacyRoster("supplied/ROSTER.DAT", raw)
	if e != nil {
		t.Fatal(e)
	}
	old := &SaveStore{Directory: dir}
	if e = old.Initialize(before); e != nil {
		t.Fatal(e)
	}
	if e = MigrateCareer(dir, "", app); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(filepath.Join(dir, "imports"), filepath.Join(app, "detached-evidence")); e != nil {
		t.Fatal(e)
	}
	s, e := OpenManagedCareer(dir, "", app, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !reflect.DeepEqual(s.State(), before) || !bytes.Equal(s.State().Legacy.RawRoster, raw) {
		t.Fatal("import identity, seventh record or pilot state differed")
	}
}

func TestFreshManagedRosterRejectsPartialImportAndPreservesEstablishedPacks(t *testing.T) {
	for _, mode := range []string{"absent", "truncated", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			app, original := t.TempDir(), t.TempDir()
			dir := filepath.Join(app, "saves")
			if mode != "absent" {
				b := syntheticLegacyRoster()
				if mode == "truncated" {
					b = b[:10]
				}
				if err := os.WriteFile(filepath.Join(original, "ROSTER.DAT"), b, 0600); err != nil {
					t.Fatal(err)
				}
				code := []byte("unsupported")
				if mode == "truncated" {
					if source := os.Getenv("GS2000_TEST_ORIGINAL"); source != "" {
						var readErr error
						code, readErr = os.ReadFile(filepath.Join(source, "GS.GS2"))
						if readErr != nil {
							t.Fatal(readErr)
						}
					}
				}
				if err := os.WriteFile(filepath.Join(original, "GS.GS2"), code, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, err := OpenManagedCareer(dir, original, app, true)
			if err != nil {
				t.Fatal(err)
			}
			before := s.State()
			template, err := defaultRoster(app)
			if err != nil {
				t.Fatal(err)
			}
			if before.Slots[0] == nil || before.Slots[0].Name != template.Slots[0].Name || before.Legacy != nil {
				t.Fatal("missing checked default roster")
			}
			s.Close()
			if _, err = os.Stat(filepath.Join(dir, "imports")); !os.IsNotExist(err) {
				t.Fatal("partial import committed", err)
			}
			if err = os.WriteFile(filepath.Join(original, "ROSTER.DAT"), []byte("changed"), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenManagedCareer(dir, original, app, true)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, reopened.State()) {
				t.Fatal("legacy source replaced existing data")
			}
			reopened.Close()
			if err = os.WriteFile(filepath.Join(dir, "player0.g2pak"), []byte("invalid port pack"), 0600); err != nil {
				t.Fatal(err)
			}
			if broken, e := OpenManagedCareer(dir, original, app, true); e == nil {
				broken.Close()
				t.Fatal("corrupt port career replaced")
			}
		})
	}
}
