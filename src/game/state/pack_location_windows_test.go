package state

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLockedSlotDuringLayoutUpgrade(t *testing.T) {
	if root := os.Getenv("GOSHIP_SHARING_ROOT"); root != "" {
		if e := PrepareExistingCareer(root, os.Getenv("GOSHIP_SHARING_CONFIG")); e != nil {
			t.Fatal(e)
		}
		return
	}
	app := t.TempDir()
	root, _ := legacyLayout(t, app)
	slot := filepath.Join(root, playerPack(0))
	dst := filepath.Join(root, "pilots", playerPack(0))
	b, e := os.ReadFile(slot)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(dst, b, 0600); e != nil {
		t.Fatal(e)
	}
	run := func() {
		t.Helper()
		c := exec.Command(os.Args[0], "-test.run=^TestLockedSlotDuringLayoutUpgrade$")
		c.Env = append(os.Environ(), "GOSHIP_SHARING_ROOT="+root, "GOSHIP_SHARING_CONFIG="+app)
		out, e := c.CombinedOutput()
		t.Log(string(out))
		if e != nil {
			t.Fatal(e)
		}
	}
	held := func(path string, share uint32, check func()) {
		t.Helper()
		ptr, e := windows.UTF16PtrFromString(path)
		if e != nil {
			t.Fatal(e)
		}
		h, e := windows.CreateFile(ptr, windows.GENERIC_READ|windows.GENERIC_WRITE, share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if e != nil {
			t.Fatal(e)
		}
		defer windows.CloseHandle(h)
		if share == 0 {
			var ov windows.Overlapped
			if e = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ov); e != nil {
				t.Fatal(e)
			}
			defer windows.UnlockFileEx(h, 0, 1, 0, &ov)
		}
		check()
	}
	held(dst, 0, func() {
		if _, e = os.ReadFile(dst); e == nil {
			t.Fatal("expected actual sharing violation")
		}
		run()
		if _, e = os.Stat(filepath.Join(root, packLocation)); !os.IsNotExist(e) {
			t.Fatal("source authority lost", e)
		}
	})
	held(slot, windows.FILE_SHARE_READ, func() {
		run()
		if _, e = os.Stat(filepath.Join(root, packLocation)); e != nil {
			t.Fatal(e)
		}
		if _, e = os.Stat(slot); e != nil {
			t.Fatal("locked original lost", e)
		}
	})
	run()
	if _, e = os.Stat(slot); !os.IsNotExist(e) {
		t.Fatal("failed retirement retry", e)
	}
	if got, e := os.ReadFile(dst); e != nil || string(got) != string(b) {
		t.Fatal("career image changed", e)
	}
}
