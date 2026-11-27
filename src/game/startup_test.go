package main

import (
	"bytes"
	"encoding/json"
	"errors"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/managedassets"
	menuui "gunship2000-assets/game/menus"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplicationDirectoryRules(t *testing.T) {
	source := t.TempDir()
	installed := t.TempDir()
	override := t.TempDir()
	os.WriteFile(filepath.Join(source, "go.mod"), []byte("module gunship2000-assets\n"), 0600)
	os.Mkdir(filepath.Join(source, "game"), 0755)
	os.WriteFile(filepath.Join(source, "game", "main.go"), []byte("package main\n"), 0600)
	os.Mkdir(filepath.Join(source, "runtime"), 0755)
	exe := filepath.Join(installed, "gs2000")
	cases := []struct{ mode, cwd, override, want string }{
		{"auto", source, "", filepath.Join(source, "runtime")}, {"installed", source, "", installed},
		{"auto", filepath.Join(source, "runtime"), "", filepath.Join(source, "runtime")}, {"auto", t.TempDir(), "", installed}, {"installed", source, override, override},
	}
	for _, c := range cases {
		got, err := applicationDirectory(c.override, c.cwd, exe, "gunship2000-assets", c.mode)
		if err != nil || got != c.want {
			t.Fatalf("%+v: %s %v", c, got, err)
		}
	}
	// A different module in cwd is not our source launch; no ancestor search.
	if got, err := applicationDirectory("", source, exe, "another-module", "auto"); err != nil || got != installed {
		t.Fatal(got, err)
	}
}

func TestOptionalConfigDefaultsPathsAndErrors(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{}`), 0600)
	p, err := loadApplication(dir, "")
	if err != nil || p.Config.WindowWidth != 1024 || p.Config.WindowHeight != 768 || p.Config.SquarePixels || p.Original != filepath.Join(dir, "original") || p.Assets != filepath.Join(dir, "assets") || p.Saves != filepath.Join(dir, "saves") {
		t.Fatal(p, err)
	}
	dir = t.TempDir()
	config := filepath.Join(dir, "config.cfg")
	os.WriteFile(config, []byte(`{"original":"own/game","assets":"prepared","saves":"progress","window_width":800,"square_pixels":true}`), 0600)
	p, err = loadApplication(dir, "alternate")
	if err != nil || p.Original != filepath.Join(dir, "own/game") || p.Assets != filepath.Join(dir, "alternate") || p.Saves != filepath.Join(dir, "progress") || p.Config.WindowHeight != 768 || !p.Config.SquarePixels {
		t.Fatal(p, err)
	}
	for _, bad := range []string{`{`, `null`, `[]`, `{} {}`, `{"window_width":1}`, `{"window_height":9000}`, `{"assets":" "}`, `{"saves":""}`} {
		dir = t.TempDir()
		config = filepath.Join(dir, "config.cfg")
		os.WriteFile(config, []byte(bad), 0600)
		if _, err = loadApplication(dir, ""); err == nil {
			t.Fatalf("accepted bad config %s", bad)
		}
	}
}

func fakeStartup(t *testing.T) (ApplicationPaths, startupServices, *int) {
	t.Helper()
	root := t.TempDir()
	p := ApplicationPaths{Directory: root, Original: filepath.Join(root, "original"), Assets: filepath.Join(root, "assets")}
	os.Mkdir(p.Original, 0755)
	count := new(int)
	services := startupServices{
		load: func(dir string) (*assetdata.OfficeAssets, error) {
			b, err := os.ReadFile(filepath.Join(dir, "ready"))
			if err != nil {
				return nil, err
			}
			m, err := assetdata.ReadLimitedFile(filepath.Join(dir, "manifest.json"), 4096)
			if err != nil {
				return nil, err
			}
			var manifest assetdata.Manifest
			if err = json.Unmarshal(m, &manifest); err != nil {
				return nil, err
			}
			if !(manifest.SchemaVersion == assetdata.PreparedManifestSchema && manifest.PreparationVersion >= 0 && manifest.PreparationVersion <= assetdata.PreparationVersion) || string(b) != "valid" {
				return nil, errors.New("incompatible/incomplete prepared outputs")
			}
			return &assetdata.OfficeAssets{Root: dir}, nil
		},
		prepare: func(_, dir string) (*assetdata.Manifest, error) {
			*count++
			if err := os.Mkdir(dir, 0755); err != nil {
				return nil, err
			}
			m := &assetdata.Manifest{SchemaVersion: assetdata.PreparedManifestSchema, Generator: assetdata.PreparationGenerator, PreparationVersion: assetdata.PreparationVersion, Result: "partial_conversion", Summary: assetdata.Counts{Entries: 2, Pictures: 1, Unsupported: 3, ConversionFailures: 1}}
			b, _ := assetdata.JsonBytes(m)
			os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600)
			os.WriteFile(filepath.Join(dir, "ready"), []byte("valid"), 0600)
			return m, nil
		}, rename: os.Rename,
	}
	return p, services, count
}

func TestManagedFirstRunPartialAndReuseWithoutOriginals(t *testing.T) {
	p, s, count := fakeStartup(t)
	var log bytes.Buffer
	if _, err := ensureOfficeAssets(p, false, &log, s); err != nil {
		t.Fatal(err)
	}
	if *count != 1 || !strings.Contains(log.String(), "office ready") {
		t.Fatal(*count, log.String())
	}
	before, _ := os.Stat(filepath.Join(p.Assets, "ready"))
	// Do not require source availability or perform writes on a usable-tree launch.
	p.Original = filepath.Join(p.Directory, "not-installed")
	s.prepare = func(string, string) (*assetdata.Manifest, error) { t.Fatal("unexpected generation"); return nil, nil }
	if _, err := ensureOfficeAssets(p, false, io.Discard, s); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(p.Assets, "ready"))
	if !before.ModTime().Equal(after.ModTime()) {
		*count = -1
		t.Fatal("reuse rewrote an output")
	}
	if _, err := os.Stat(p.Assets + ".prepare.lock"); !os.IsNotExist(err) {
		t.Fatal("fast path created a lock")
	}
}

func TestManagedRecoverIncompleteAndIncompatibleOutputs(t *testing.T) {
	for _, kind := range []string{"missing-output", "future-version"} {
		t.Run(kind, func(t *testing.T) {
			p, s, count := fakeStartup(t)
			ensureOfficeAssets(p, false, io.Discard, s)
			if kind == "missing-output" {
				os.Remove(filepath.Join(p.Assets, "ready"))
			} else {
				b, _ := assetdata.JsonBytes(assetdata.Manifest{SchemaVersion: assetdata.PreparedManifestSchema, Generator: assetdata.PreparationGenerator, PreparationVersion: assetdata.PreparationVersion + 1})
				os.WriteFile(filepath.Join(p.Assets, "manifest.json"), b, 0600)
			}
			if _, err := ensureOfficeAssets(p, false, io.Discard, s); err != nil {
				t.Fatal(err)
			}
			if *count != 2 {
				t.Fatal(*count)
			}
			backups, _ := filepath.Glob(filepath.Join(p.Directory, ".assets.previous-*", "tree", "manifest.json"))
			if len(backups) != 1 {
				t.Fatal("old tree not preserved", backups)
			}
		})
	}
}

func TestManagedFailureKeepsPreviousAndAttempt(t *testing.T) {
	for _, failure := range []string{"generation", "validation", "promotion"} {
		t.Run(failure, func(t *testing.T) {
			p, s, _ := fakeStartup(t)
			ensureOfficeAssets(p, false, io.Discard, s)
			os.WriteFile(filepath.Join(p.Assets, "previous-only"), []byte("retain"), 0600)
			prepare := s.prepare
			if failure == "generation" {
				s.prepare = func(a, b string) (*assetdata.Manifest, error) {
					prepare(a, b)
					return nil, errors.New("synthetic generation failure")
				}
			}
			if failure == "validation" {
				s.prepare = func(a, b string) (*assetdata.Manifest, error) {
					m, e := prepare(a, b)
					os.WriteFile(filepath.Join(b, "ready"), []byte("invalid"), 0600)
					return m, e
				}
			}
			if failure == "promotion" {
				s.rename = func(a, b string) error {
					if strings.Contains(a, ".preparing-") {
						return errors.New("synthetic rename failure")
					}
					return os.Rename(a, b)
				}
			}
			if _, err := ensureOfficeAssets(p, true, io.Discard, s); err == nil {
				t.Fatal("failure reported success")
			}
			if b, err := os.ReadFile(filepath.Join(p.Assets, "previous-only")); err != nil || string(b) != "retain" {
				t.Fatal("previous tree lost", err)
			}
			attempts, _ := filepath.Glob(filepath.Join(p.Directory, ".assets.preparing-*", "tree"))
			if len(attempts) != 1 {
				t.Fatal("failed attempt lost", attempts)
			}
			if _, err := s.load(p.Assets); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestManagedRefusesUnrelatedOverlapSymlinkAndLock(t *testing.T) {
	p, s, count := fakeStartup(t)
	os.Mkdir(p.Assets, 0755)
	os.WriteFile(filepath.Join(p.Assets, "personal"), []byte("safe"), 0600)
	if _, err := ensureOfficeAssets(p, false, io.Discard, s); err == nil || *count != 0 {
		t.Fatal("unrelated directory accepted", err)
	}
	for _, target := range []string{p.Original, filepath.Join(p.Original, "inside"), p.Directory} {
		if _, _, err := managedPaths(p.Original, target); err == nil {
			t.Fatal("overlap accepted", target)
		}
	}
	link := filepath.Join(p.Directory, "link")
	if err := os.Symlink(p.Assets, link); err == nil {
		if _, _, err = managedPaths(p.Original, link); err == nil {
			t.Fatal("replacement symlink accepted")
		}
	}
	q, _, _ := fakeStartup(t)
	os.WriteFile(q.Assets+".prepare.lock", []byte("other process"), 0600)
	if owner, err := configuration.BeginOwnership(q.Directory, "installed", "", false); err == nil {
		owner.Close()
		t.Fatal("concurrent lock accepted")
	}
	b, _ := os.ReadFile(q.Assets + ".prepare.lock")
	if string(b) != "other process" {
		t.Fatal("foreign lock changed")
	}
}

func TestInterruptedPromotionRestoresPreviousTree(t *testing.T) {
	p, s, _ := fakeStartup(t)
	ensureOfficeAssets(p, false, io.Discard, s)
	backup, err := os.MkdirTemp(p.Directory, ".assets.previous-")
	if err != nil {
		t.Fatal(err)
	}
	os.Rename(p.Assets, filepath.Join(backup, "tree"))
	b, _ := assetdata.JsonBytes(preparationRecovery{1, "assets", filepath.Base(backup)})
	os.WriteFile(p.Assets+".prepare-recovery.json", b, 0600)
	s.prepare = func(string, string) (*assetdata.Manifest, error) {
		t.Fatal("restored assets should be reused")
		return nil, nil
	}
	if _, err := ensureOfficeAssets(p, false, io.Discard, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.Assets + ".prepare-recovery.json"); !os.IsNotExist(err) {
		t.Fatal("journal not reconciled")
	}
}

func TestLegacyPreparedVersionAndManagedIdentification(t *testing.T) {
	if !(assetdata.PreparedManifestSchema == 1) {
		t.Fatal("legacy Step21 no longer accepted")
	}
	if assetdata.PreparationVersion >= 999 {
		t.Fatal("future preparation accepted")
	}
	p, s, _ := fakeStartup(t)
	ensureOfficeAssets(p, false, io.Discard, s)
	b, _ := assetdata.JsonBytes(assetdata.Manifest{SchemaVersion: 1})
	os.WriteFile(filepath.Join(p.Assets, "manifest.json"), b, 0600)
	if err := replaceableAssets(p.Assets); err == nil {
		t.Fatal("unidentified legacy tree replaceable")
	}
}

func TestManagedRealInstallationWhenRequested(t *testing.T) {
	original, output := os.Getenv("GS2000_STARTUP_ORIGINAL"), os.Getenv("GS2000_STARTUP_OUTPUT")
	if original == "" || output == "" {
		t.Skip("explicit integration paths required")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("integration output must not exist")
	}
	p := ApplicationPaths{Directory: filepath.Dir(output), Original: original, Assets: output}
	s := defaultStartupServices()
	if result := startOffice(p, false, os.Stdout, s, func(o *menuui.Office) error {
		if len(o.Scene) != 64000 {
			return errors.New("incomplete office scene")
		}
		return nil
	}); result != 0 {
		t.Fatal("first-run readiness failed")
	}
	p.Original = filepath.Join(p.Directory, "originals-removed-for-reuse-check")
	if _, err := ensureOfficeAssets(p, false, os.Stdout, s); err != nil {
		t.Fatal(err)
	}
}

// Exercise normal source selection and publication with a lightweight prepared
// output. The discovery/signature tests cover the provider used by the game.
func TestPreparationPublishesSourceOnlyAfterValidation(t *testing.T) {
	p, fixture, _ := fakeStartup(t)
	p.Config.Original = "original"
	p.Config.OriginalSource = configuration.OriginalUnselected
	candidate := filepath.Join(t.TempDir(), "External Game")
	if err := os.Mkdir(candidate, 0700); err != nil {
		t.Fatal(err)
	}
	selected := ""
	services := managedassets.Services{
		Installations: func() []managedassets.Installation { return nil },
		Prepare: func(input, output string) (*assetdata.Manifest, error) {
			if input != candidate {
				t.Fatalf("wrong preparation input %q", input)
			}
			return fixture.prepare(input, output)
		},
		Load:   fixture.load,
		Rename: fixture.rename,
		SourceUsed: func(choice string) {
			if _, err := fixture.load(p.Assets); err != nil {
				t.Fatalf("choice published before assets were readable: %v", err)
			}
			selected = choice
		},
	}
	run := func() error { _, err := managedassets.Ensure(sharedPaths(p), false, io.Discard, services); return err }
	err := run()
	if !errors.Is(err, managedassets.ErrOriginalUnavailable) || !strings.Contains(err.Error(), p.Original) || !strings.Contains(err.Error(), "Port Settings") {
		t.Fatalf("missing location prompt: %v", err)
	}
	services.Installations = func() []managedassets.Installation {
		return []managedassets.Installation{{Source: configuration.OriginalSteam, Path: candidate}}
	}
	services.Ready = func(*assetdata.OfficeAssets) error { return errors.New("candidate validation failed") }
	if err = run(); err == nil || selected != "" {
		t.Fatal("invalid source published", err, selected)
	}
	services.Ready = nil
	if err = run(); err != nil || selected != candidate {
		t.Fatal("validated source missing", err, selected)
	}
	services.Installations = func() []managedassets.Installation { t.Fatal("reused cache invoked discovery"); return nil }
	if err = run(); err != nil {
		t.Fatal(err)
	}
}
