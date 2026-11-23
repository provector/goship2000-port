package main

import (
	"bytes"
	"gunship2000-assets/internal/testfixture"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissionPlayerStartupReadinessWithoutOriginals(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("explicit prepared mission witness required")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	paths := ApplicationPaths{Directory: t.TempDir(), Assets: dir, Original: filepath.Join(t.TempDir(), "missing-original")}
	assets, err := ensureOfficeAssets(paths, false, &bytes.Buffer{}, frontEndStartupServices())
	if err != nil || assets == nil || assets.Missions == nil || assets.Menus == nil || assets.Startup == nil {
		t.Fatal("normal startup cannot reuse complete tree without originals", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("reuse rewrote prepared assets", err)
	}
	// Retain the exact development failure as an upgrade regression: a tree can
	// have all pictures and still lack the original footer-caption font.
	old := testfixture.Path("prepared-missions")
	if _, err = os.Stat(old); err == nil {
		if _, err = frontEndStartupServices().load(old); err == nil || !(strings.Contains(err.Error(), "font5") || strings.Contains(err.Error(), "incompatible mission preparation metadata")) {
			t.Fatal("incomplete tree no longer reports its missing dependency", err)
		}
	}
}
