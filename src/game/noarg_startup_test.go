package main

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/userpack"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This gate compiles the ordinary production entry and reaches it without any
// game arguments. The headless build deliberately stops AFTER actual preparation,
// capability/readback and SaveStore entry; connected final-package UI is separate.
func TestSourceEquivalentNoArgumentPreparation(t *testing.T) {
	original, config := os.Getenv("GS2000_STARTUP_ORIGINAL"), os.Getenv("GS2000_STARTUP_CONFIG")
	if original == "" || config == "" {
		t.Skip("mandatory release prerequisite: original installation and normal runtime config; set GS2000_STARTUP_ORIGINAL and GS2000_STARTUP_CONFIG")
	}
	raw, e := os.ReadFile(config)
	if e != nil {
		t.Fatal(e)
	}
	var selected map[string]json.RawMessage
	if e = json.Unmarshal(raw, &selected); e != nil {
		t.Fatal(e)
	}
	var source string
	if e = json.Unmarshal(selected["original"], &source); e != nil || filepath.Clean(source) != filepath.Join("..", "original") {
		t.Fatalf("source-equivalent fixture requires unchanged original ../original, got %q: %v", source, e)
	}
	for _, key := range []string{"assets", "saves"} {
		if b, ok := selected[key]; ok {
			var s string
			if json.Unmarshal(b, &s) != nil || s != key {
				t.Fatalf("unsafe startup fixture %s=%s", key, b)
			}
		}
	}
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	module := filepath.Dir(cwd)
	root := t.TempDir()
	sourceModule := filepath.Join(root, "src")
	app := filepath.Join(sourceModule, "runtime")
	for _, d := range []string{app, filepath.Join(sourceModule, "game"), filepath.Join(sourceModule, "original")} {
		if e = os.MkdirAll(d, 0700); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"go.mod", "game/main.go"} {
		b, e := os.ReadFile(filepath.Join(module, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(sourceModule, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.WriteFile(filepath.Join(app, "config.cfg"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	nested := filepath.Join(sourceModule, "original", "GS2000CD")
	if e = copyPPreparedFixture(original, nested); e != nil {
		t.Fatal(e)
	}
	binary := os.Getenv("GS2000_TEST_HEADLESS_ENTRY")
	if binary == "" {
		goexe := os.Getenv("GS2000_TEST_GO")
		if goexe == "" {
			goexe = filepath.Join(runtime.GOROOT(), "bin", "go")
		}
		binary = filepath.Join(root, "source-entry")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		build := exec.Command(goexe, "build", "-tags", "headless", "-o", binary, "./game")
		build.Dir = module
		if out, e := build.CombinedOutput(); e != nil {
			t.Fatalf("build ordinary entry: %v\n%s", e, out)
		}
	}

	for _, name := range []string{"saves", "films", "missions"} {
		if e = os.MkdirAll(filepath.Join(app, name), 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(app, name, "preserve.txt"), []byte(name), 0600); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = os.Stat(filepath.Join(app, "assets")); !os.IsNotExist(e) {
		t.Fatal("missing-cache case was prefilled", e)
	}
	for iteration := 0; iteration < 2; iteration++ {
		command := exec.Command(binary)
		command.Dir = app
		// The fixture owns both the development installation and its player data.
		command.Env = append(os.Environ(), "GOSHIP2000_DATA_DIR="+app)
		if len(command.Args) != 1 {
			t.Fatal("startup gate acquired hidden arguments")
		}
		out, e := command.CombinedOutput()
		if e == nil || !bytes.Contains(out, []byte("this binary was built with -tags headless")) || !bytes.Contains(out, []byte("Player packs: "+filepath.Join(app, "saves"))) {
			t.Fatalf("ordinary entry didnot reach post-readiness headless boundary: %v\n%s", e, out)
		}
		preparing := bytes.Contains(out, []byte("Preparing assets from "+filepath.Join(sourceModule, "original")))
		if preparing != (iteration == 0) {
			t.Fatalf("unexpected preparation iteration%d: %s", iteration, out)
		}
		t.Logf("no arguments; cwd=%s; iteration=%d; preparation=%v; post-readiness headless boundary reached", app, iteration, preparing)
	}
	assets, e := assetdata.LoadFrontEndAssets(filepath.Join(app, "assets"))
	if e != nil {
		t.Fatal(e)
	}
	if e = assetdata.ValidateCurrentFrontEndCapabilities(assets); e != nil {
		t.Fatal(e)
	}
	for name, destination := range map[string]string{"saves": "saves", "films": "saves/replays", "missions": "saves/missions"} {
		b, e := os.ReadFile(filepath.Join(app, filepath.FromSlash(destination), "preserve.txt"))
		if e != nil || string(b) != name {
			t.Fatal("runtime sentinel changed", name, e)
		}
	}
	after, e := userpack.ReadConfig(app, "settings.json")
	if e != nil || !bytes.Equal(raw, after) {
		t.Fatal("normal config changed", e)
	}
	if matches, _ := filepath.Glob(filepath.Join(app, ".assets.preparing-*")); len(matches) != 0 {
		t.Fatal("successful preparation retained incomplete attempt", matches)
	}
	if _, e = os.Stat(filepath.Join(app, "assets.prepare.lock")); !os.IsNotExist(e) {
		t.Fatal("preparation lock remains", e)
	}
	// This source-equivalent context intentionally keeps the exact relative source
	// setting. Absolute originals/asset overrides would be a different test.
	if !strings.HasPrefix(nested, root+string(os.PathSeparator)) {
		t.Fatal("unsafe owned layout")
	}
}
