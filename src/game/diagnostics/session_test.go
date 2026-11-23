package diagnostics

import (
	"archive/zip"
	"fmt"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/internal/branding"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readReport(t *testing.T, path string) map[string]string {
	t.Helper()
	z, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	out := map[string]string{}
	for _, v := range z.File {
		r, e := v.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		out[v.Name] = string(b)
	}
	return out
}

func TestSessionDeathRecovery(t *testing.T) {
	if root := os.Getenv("G2_TEST_SESSION_ROOT"); root != "" {
		owner, e := configuration.OwnOutput(root)
		if e != nil {
			t.Fatal(e)
		}
		defer owner.Close()
		s, e := StartSession(root, "child", "disabled text and terminal")
		if e != nil {
			t.Fatal(e)
		}
		Info("orders", "retained order before interruption")
		if _, e = openRotating(root, "child-log.txt", 1024); e != nil {
			t.Fatal(e)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			data, _ := os.ReadFile(filepath.Join(s.dir, "journal.log"))
			if strings.Contains(string(data), "retained order") {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("journal never flushed")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if e = os.WriteFile(filepath.Join(root, "ready"), []byte("ready"), 0600); e != nil {
			t.Fatal(e)
		}
		if os.Getenv("G2_TEST_SESSION_PANIC") == "true" {
			panic("U2.6 disposable panic witness")
		}
		for {
			time.Sleep(time.Minute)
		}
	}
	for _, cause := range []string{"kill", "panic"} {
		t.Run(cause, func(t *testing.T) {
			root := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestSessionDeathRecovery$")
			cmd.Env = append(os.Environ(), "G2_TEST_SESSION_ROOT="+root, fmt.Sprintf("G2_TEST_SESSION_PANIC=%t", cause == "panic"))
			if e := cmd.Start(); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { cmd.Process.Kill() })
			end := time.Now().Add(10 * time.Second)
			for {
				if _, e := os.Stat(filepath.Join(root, "ready")); e == nil {
					break
				}
				if time.Now().After(end) {
					t.Fatal("child timeout")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if cause == "kill" {
				if owner, e := configuration.OwnOutput(root); e == nil {
					owner.Close()
					t.Fatal("second process acquired the live store")
				}
				if found, _ := filepath.Glob(filepath.Join(root, "reports", "*", "crash_report.zip")); len(found) != 0 {
					t.Fatal("reported live owner")
				}
				if e := cmd.Process.Kill(); e != nil {
					t.Fatal(e)
				}
			}
			if cmd.Wait() == nil {
				t.Fatal("child did not terminate unexpectedly")
			}
			owner, e := configuration.OwnOutput(root)
			if e != nil {
				t.Fatal(e)
			}
			defer owner.Close()
			s, e := StartSession(root, "next", "")
			if e != nil {
				t.Fatal(e)
			}
			s.Close()
			files, _ := filepath.Glob(filepath.Join(root, "reports", "*", "crash_report.zip"))
			if len(files) != 1 {
				t.Fatal(files)
			}
			contents := readReport(t, files[0])
			if !strings.Contains(contents["log.txt"], "retained order") || !strings.Contains(contents["recovery.json"], "recording") {
				t.Fatal(contents)
			}
			if (cause == "panic") != strings.Contains(contents["crash.txt"], "disposable panic witness") {
				t.Fatal("unexpected fatal output", contents["crash.txt"])
			}
			f, e := openRotating(root, "child-log.txt", 1024)
			if e != nil {
				t.Fatal(e)
			}
			f.Close()
		})
	}
}
func TestHandledReportBoundedAndFailureIsolated(t *testing.T) {
	root := t.TempDir()
	s, e := StartSession(root, "build", "both off")
	if e != nil {
		t.Fatal(e)
	}
	line := strings.Repeat("x", MaxRecordBytes-1) + "\n"
	for i := 0; i < 2000; i++ {
		recordHistory(line)
	}
	Info("career", "reached commit")
	if s.ring.bytes > historyBudget {
		t.Fatal("unbounded")
	}
	Report("one", "distinct failure", "Your mission result was saved.")
	Report("one", "same incident", "duplicate")
	s.Close()
	files, _ := filepath.Glob(filepath.Join(root, "reports", "*", "incident-*", "error_report.zip"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	data := readReport(t, files[0])
	if !strings.Contains(data["report.txt"], "GoShip 2000 "+branding.Version+"\nFlavor "+branding.Flavor+"\n") {
		t.Fatal("missing application identity in incident report")
	}
	if len(data["log.txt"]) > historyBudget || !strings.Contains(data["log.txt"], "reached commit") {
		t.Fatal("bad recent history")
	}
	s, e = StartSession(root, "next", "")
	if e != nil {
		t.Fatal(e)
	}
	s.wg.Wait()
	bad := filepath.Join(s.dir, "regular-file")
	if e = os.WriteFile(bad, nil, 0600); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if s.makeReport(bad, s.dir, "error_report.zip", reportRequest{}, nil) {
			t.Fatal("false archive success")
		}
	}
	if msg := TakeNotice(); !strings.Contains(msg, "failed") {
		t.Fatal(msg)
	}
	if msg := TakeNotice(); msg != "" {
		t.Fatal("recursive notice", msg)
	}
	s.Close()
}

func TestUnavailableReportStorageStillKeepsHistory(t *testing.T) {
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, "reports"), []byte("existing file"), 0600); e != nil {
		t.Fatal(e)
	}
	s, e := StartSession(root, "test", "disabled")
	if e == nil || s == nil {
		t.Fatal("missing memory fallback")
	}
	defer s.Close()
	Info("flight", "control transferred")
	if Report("fault", "test", "test") {
		t.Fatal("claimed durable report")
	}
	if !strings.Contains(strings.Join(s.ring.lines, ""), "control transferred") {
		t.Fatal("optional output failure disabled memory history")
	}
}
func BenchmarkDiagnosticTransition(b *testing.B) {
	s, e := StartSession(b.TempDir(), "bench", "both optional sinks off")
	if e != nil {
		b.Fatal(e)
	}
	b.Cleanup(s.Close)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("flight", "Control ownership changed", "step", 256)
	}
}

func TestLegacyPIDResidueAndCleanDebriefWarning(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if e := cmd.Run(); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(root, "logs"), 0700); e != nil {
		t.Fatal(e)
	}
	legacy := filepath.Join(root, "logs", configuration.DefaultLogging().FileName+".lock")
	if e := os.WriteFile(legacy, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0600); e != nil {
		t.Fatal(e)
	}
	writer, e := configuration.BeginOwnership(root, "installed", "", false)
	if e != nil {
		t.Fatal(e)
	}
	writer.Close()
	for _, residue := range []string{fmt.Sprintf("%d\n", os.Getpid()), "unknown\n"} {
		if e = os.WriteFile(legacy, []byte(residue), 0600); e != nil {
			t.Fatal(e)
		}
		if f, err := configuration.BeginOwnership(root, "installed", "", false); err == nil {
			f.Close()
			t.Fatal("claimed ambiguous/live ownership")
		}
		data, err := os.ReadFile(legacy)
		if err != nil || string(data) != residue {
			t.Fatal("changed another owner")
		}
	}
	s, e := StartSession(root, "a", "off")
	if e != nil {
		t.Fatal(e)
	}
	id := strings.Repeat("2", 32)
	SetMission("world", "", "mission end", 100)
	RememberWarning(id, "held route")
	s.Close()
	next, e := StartSession(root, "b", "off")
	if e != nil {
		t.Fatal(e)
	}
	if w := RecallWarning(id); w != "held route" {
		t.Fatal(w)
	}
	if next.context.Step != 100 || next.context.Mission != "world" {
		t.Fatal(next.context)
	}
	if Report(id, "held route", "Your mission result was saved.") {
		ForgetWarning(id)
	} else {
		t.Fatal("request failed")
	}
	next.Close()
	if _, err := os.Stat(filepath.Join(root, "reports", "pending-"+id+".json")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func BenchmarkDiagnosticDisabledDebugGate(b *testing.B) {
	s, e := StartSession(b.TempDir(), "bench", "both optional sinks off")
	if e != nil {
		b.Fatal(e)
	}
	b.Cleanup(s.Close)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if Enabled(slog.LevelDebug) {
			b.Fatal("DEBUG on")
		}
	}
}
