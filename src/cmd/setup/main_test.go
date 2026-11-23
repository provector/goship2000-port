package main

import (
	"gunship2000-assets/game/configuration"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupExplicitInitializeAndCheckNoAssets(t *testing.T) {
	args := os.Args
	defer func() { os.Args = args }()
	dir := t.TempDir()
	os.Args = []string{"setup", "-app-dir", dir, "-init"}
	if code := run(); code != 0 {
		t.Fatal(code)
	}
	p := filepath.Join(dir, configuration.Filename)
	before, _ := os.ReadFile(p)
	os.Args = []string{"setup", "-app-dir", dir, "-check"}
	if code := run(); code != 0 {
		t.Fatal(code)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("check rewrote cfg")
	}
	for _, n := range []string{"assets", "saves", "films", "missions"} {
		if _, e := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(e) {
			t.Fatal(n, e)
		}
	}
}
