package main

import (
	"os"
	"path/filepath"
	"testing"
)

func testSaveBytes(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(dir, name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
