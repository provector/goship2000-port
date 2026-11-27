package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMacPathAliasesAndDataChoice(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOSHIP2000_DATA_DIR", "")
	want := filepath.Join(home, "Library", "Application Support", "GoShip 2000", "Data")
	if p, e := DataChoice("/installation", ""); e != nil || p != want {
		t.Fatal(p, e)
	}
	t.Setenv("GOSHIP2000_DATA_DIR", filepath.Join(home, "moved"))
	if p, e := DataChoice("/installation", ""); e != nil || p != os.Getenv("GOSHIP2000_DATA_DIR") {
		t.Fatal(p, e)
	}
	if !Within("/tmp/Café/SAVES/pilots", "/tmp/Cafe\u0301/saves") || Within("/tmp/Save2", "/tmp/save") {
		t.Fatal("aliased data-tree overlap")
	}
}
