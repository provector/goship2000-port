package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordingCLIRejectsBeforeOpeningApplication(t *testing.T) {
	for _, arg := range []string{"0", "-1", "257", "1.5", "999999999999999999999999999"} {
		root := filepath.Join(t.TempDir(), "must-not-exist")
		if code := runNormal([]string{"-app-dir", root, "-replay-max-mib", arg}); code == 0 {
			t.Fatal("invalid admitted", arg)
		}
		if _, e := os.Stat(root); !os.IsNotExist(e) {
			t.Fatal("invalid policy opened application")
		}
	}
}
