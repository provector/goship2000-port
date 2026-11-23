//go:build linux

package replay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// The file-size limit applies only to this test's subprocess. The real kernel
// write failure exercises production Seal; the parent suite never changes its
// global limits or signals and no filesystem/service mock supplies the error.
func TestSealActualPartialWritePreservesPrefix(t *testing.T) {
	if os.Getenv("GS2000_RLIMIT_SEAL_CHILD") != "1" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "-test.run=^TestSealActualPartialWritePreservesPrefix$", "-test.v")
		cmd.Env = append(os.Environ(), "GS2000_RLIMIT_SEAL_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated real I/O failure: %v\n%s", err, out)
		}
		if !bytes.Contains(out, []byte("partial_write=1024")) || !bytes.Contains(out, []byte("phase=seal-staging")) {
			t.Fatalf("missing actual partial-write/phase receipt:\n%s", out)
		}
		t.Logf("isolated RLIMIT_FSIZE child: %s", out)
		return
	}
	f := testFilm(strings.Repeat("a", 32))
	r, err := (Store{Root: t.TempDir()}).BeginTemporary(f)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	op := Operation{Kind: "boundary", Payload: json.RawMessage(`{"Kind":"reset","Diagnostic":"` + strings.Repeat("x", 32768) + `"}`), Checkpoint: map[string]string{"flight": strings.Repeat("b", 64)}}
	if err = r.Append(op); err != nil {
		t.Fatal(err)
	}
	f.Operations = append(f.Operations, op)
	if err = r.flushBatch(); err != nil {
		t.Fatal(err)
	}
	if err = r.buffer.Flush(); err != nil {
		t.Fatal(err)
	}
	if err = r.file.Sync(); err != nil {
		t.Fatal(err)
	}
	prefix, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	var prior syscall.Rlimit
	if err = syscall.Getrlimit(syscall.RLIMIT_FSIZE, &prior); err != nil {
		t.Fatal(err)
	}
	signal.Ignore(syscall.SIGXFSZ)
	if err = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 1024, Max: prior.Max}); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_FSIZE, &prior)
	// A direct owned-file receipt proves this environment performs an actual
	// partial regular-file write rather than merely rejecting file creation.
	probe, err := os.Create(filepath.Join(filepath.Dir(r.path), "rlimit-probe"))
	if err != nil {
		t.Fatal(err)
	}
	n, writeErr := probe.Write(make([]byte, 32768))
	probe.Close()
	if n != 1024 || writeErr == nil {
		t.Fatalf("kernel partial write=%d error=%v", n, writeErr)
	}
	// Force a partial write of the bounded index/ending after the last verified frame.
	if err = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: uint64(len(prefix) + 24), Max: prior.Max}); err != nil {
		t.Fatal(err)
	}
	err = r.Seal(f)
	if err == nil || RecordingFailurePhase(err, "") != "seal-staging" {
		t.Fatal("real stage write did not fail", err)
	}
	after, e := os.ReadFile(r.path)
	if e != nil || !bytes.Equal(prefix, after) {
		t.Fatal("only valid prefix changed after actual partial write", e)
	}
	if _, e = os.Stat(r.path + ".sealed"); !os.IsNotExist(e) {
		t.Fatal("partial stage presented as a sealed film", e)
	}
	stages, e := filepath.Glob(filepath.Join(filepath.Dir(r.path), ".film-sealing-*"))
	if e != nil || len(stages) != 0 {
		t.Fatal("failed private stage retained as a candidate", stages, e)
	}
	fmt.Printf("partial_write=%d phase=%s prefix_bytes=%d preserved=true\n", n, RecordingFailurePhase(err, ""), len(prefix))
}
