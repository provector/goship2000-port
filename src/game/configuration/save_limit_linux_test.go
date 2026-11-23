package configuration

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
)

// Limit only a disposable child's file size. A failed pack write must report
// the error and leave the entire previous pack available to the next opener.
func TestSaveRefusedByFilesystemPreservesPreviousPack(t *testing.T) {
	if os.Getenv("GOSHIP_TEST_LIMIT_CONFIG_WRITE") == "1" {
		d, err := Load(Filename)
		if err != nil {
			t.Fatal(err)
		}
		d.Config.ReplayMaxMiB = 321
		signal.Ignore(syscall.SIGXFSZ)
		var lim syscall.Rlimit
		if err = syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
			t.Fatal(err)
		}
		lim.Cur = 512
		if err = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
			t.Fatal(err)
		}
		if err = d.Save(); !errors.Is(err, syscall.EFBIG) {
			t.Fatalf("save under the 512-byte file limit returned %v", err)
		}
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	d, _, err := Initialize(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil || len(before) <= 512 || d.Config.ReplayMaxMiB == 321 {
		t.Fatal("unexpected initial pack", len(before), err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestSaveRefusedByFilesystemPreservesPreviousPack$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOSHIP_TEST_LIMIT_CONFIG_WRITE=1")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("limited child: %v: %s", err, b)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed save changed the previous pack", err)
	}
	reopened, err := Load(path)
	if err != nil || reopened.Config.ReplayMaxMiB != d.Config.ReplayMaxMiB {
		t.Fatal("prior setting was not available on reopen", err)
	}
	reopened.Config.ReplayMaxMiB = 321
	if err = reopened.Save(); err != nil {
		t.Fatal("subsequent normal save failed", err)
	}
}
