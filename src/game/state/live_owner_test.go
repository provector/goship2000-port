package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/configuration"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The controller parent is a competing process. Pipe barriers put the kill
// after entry, after staging or after the journal's durable commit decision.
func TestLiveCareerProcessLifetime(t *testing.T) {
	if dir := os.Getenv("GS2000_TEST_LEASE_CHILD"); dir != "" {
		owner, e := configuration.OwnOutput(dir)
		if e != nil {
			t.Fatal(e)
		}
		defer owner.Close()
		s, e := OpenLiveSaveStore(dir, "")
		if e != nil {
			t.Fatal(e)
		}
		pendingTestPilot(t, s)
		start, e := s.BeginSortie(pendingTestStart())
		if e != nil {
			t.Fatal(e)
		}
		mode := os.Getenv("GS2000_TEST_LEASE_STAGE")
		if mode != "flight" {
			if e = s.StageSortieResult(start.ID, pendingTestProposal(t, s, start)); e != nil {
				t.Fatal(e)
			}
		}
		if mode == "intent" {
			j := pendingCommitJournal(t, s, start.ID)
			b, e := json.Marshal(j)
			if e != nil {
				t.Fatal(e)
			}
			if e = writeSaveAtomic(dir, saveJournal, b); e != nil {
				t.Fatal(e)
			}
		}
		fmt.Fprintln(os.Stdout, start.ID)
		var next [1]byte
		if _, e = io.ReadFull(os.Stdin, next[:]); e != nil {
			t.Fatal(e)
		}
		if e = s.CommitSortie(start.ID); e != nil {
			t.Fatal(e)
		}
		if e = s.CommitSortie(start.ID); e != nil {
			t.Fatal(e)
		}
		if e = s.Close(); e != nil {
			t.Fatal(e)
		}
		return
	}
	for _, mode := range []string{"flight", "debrief", "commit", "intent"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestLiveCareerProcessLifetime$")
			cmd.Env = append(os.Environ(), "GS2000_TEST_LEASE_CHILD="+dir, "GS2000_TEST_LEASE_STAGE="+mode)
			in, e := cmd.StdinPipe()
			if e != nil {
				t.Fatal(e)
			}
			out, e := cmd.StdoutPipe()
			if e != nil {
				t.Fatal(e)
			}
			cmd.Stderr = os.Stderr
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() {
				in.Close()
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			scan := bufio.NewScanner(out)
			if !scan.Scan() || len(scan.Text()) != 32 {
				t.Fatal("child failed before barrier", scan.Text(), scan.Err())
			}
			for _, path := range []string{dir + string(filepath.Separator) + ".", filepath.Join(dir, saveJournal)} {
				if f, e := configuration.OwnOutput(path); e == nil {
					f.Close()
					t.Fatal("competing recovery admitted")
				}
			}
			other, e := configuration.OwnOutput(t.TempDir())
			if e != nil {
				t.Fatal(e)
			}
			other.Close()
			if mode == "commit" {
				if _, e = in.Write([]byte{'c'}); e != nil {
					t.Fatal(e)
				}
				if e = cmd.Wait(); e != nil {
					t.Fatal(e)
				}
			} else {
				if e = cmd.Process.Kill(); e != nil {
					t.Fatal(e)
				}
				cmd.Wait()
			}
			owner, e := configuration.OwnOutput(dir)
			if e != nil {
				t.Fatal(e)
			}
			defer owner.Close()
			reopened, e := OpenLiveSaveStore(dir, "")
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			want := int16(0)
			if mode == "commit" || mode == "intent" {
				want = 30
			}
			if got := reopened.State().SelectedPilot().Statistics.CareerScore; int(got) != int(want) {
				t.Fatalf("career score %v, want %v", got, want)
			}
		})
	}
}
