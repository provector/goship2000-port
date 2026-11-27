package configuration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCareerJunctionResolvesToTargetVolumeAndSharesOwnership(t *testing.T) {
	app := t.TempDir()
	dir, err := preferredData()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("default Documents resolver", dir)
	dst := filepath.Join(app, "données")
	if err = os.Mkdir(dst, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(app, "alias")
	if b, e := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", alias, dst).CombinedOutput(); e != nil {
		t.Fatal(string(b), e)
	}
	real := SavePath(app, app, alias)
	want, err := ResolvedNewPath(dst)
	if err != nil || real != want {
		t.Fatal(real, want, err)
	}
	own, e := OwnOutput(dst)
	if e != nil {
		t.Fatal(e)
	}
	defer own.Close()
	if f, err := OwnOutput(filepath.Join(alias, "replays", "a.g2r")); err == nil {
		f.Close()
		t.Fatal("junction bypassed writer")
	}
}
