package diagnostics

import (
	"bytes"
	"gunship2000-assets/game/configuration"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisabledAndIndependentDestinations(t *testing.T) {
	root := t.TempDir()
	c := configuration.DefaultLogging()
	if r := Start(root, c, Options{}); r != nil {
		t.Fatal("off allocated logger")
	}
	Info("test", "off")
	if _, e := os.Stat(filepath.Join(root, "logs")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	c.Level = "INFO"
	var console bytes.Buffer
	c.ShowLog = true
	r := Start(root, c, Options{Console: &console})
	Info("test", "console-only")
	Debug("test", "hidden")
	r.Close()
	if !strings.Contains(console.String(), "console-only") || strings.Contains(console.String(), "hidden") {
		t.Fatal(console.String())
	}
	if _, e := os.Stat(filepath.Join(root, "logs")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	c.ShowLog = false
	c.WriteLog = true
	c.Level = "WARN"
	r = Start(root, c, Options{})
	Info("test", "filtered")
	Warn("test", "retained")
	r.Close()
	b, _ := os.ReadFile(configuration.LogPath(root, c))
	if strings.Contains(string(b), "filtered") || !strings.Contains(string(b), "retained") {
		t.Fatal(string(b))
	}
	old := len(b)
	r = Start(root, c, Options{})
	Error("test", "second-run")
	r.Close()
	b, _ = os.ReadFile(configuration.LogPath(root, c))
	if len(b) <= old || !strings.Contains(string(b), "second-run") {
		t.Fatal("did not append")
	}
}
func TestRotationExactBoundaryAndOwnership(t *testing.T) {
	root := t.TempDir()
	cap := int64(len(fileHeader) + 20)
	r, e := openRotating(root, "bounded.txt", cap)
	if e != nil {
		t.Fatal(e)
	}
	r.Write([]byte(strings.Repeat("A", 20)))
	if _, e = os.Stat(r.path + ".previous"); !os.IsNotExist(e) {
		t.Fatal("rotated at equality")
	}
	r.Write([]byte("B"))
	r.Close()
	a, _ := os.ReadFile(r.path)
	p, _ := os.ReadFile(r.path + ".previous")
	if string(a) != fileHeader+"B" || string(p) != fileHeader+strings.Repeat("A", 20) {
		t.Fatal(string(a), string(p))
	}
	os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("precious"), 0600)
	if _, e = openRotating(root, "unrelated.txt", 100); e == nil {
		t.Fatal("overwrote foreign file")
	}
	b, _ := os.ReadFile(filepath.Join(root, "unrelated.txt"))
	if string(b) != "precious" {
		t.Fatal("changed foreign file")
	}
	held, err := openRotating(root, "bounded.txt", cap)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close() // lock exclusion tested below on this owned open file.
	if _, e = openRotating(root, "bounded.txt", cap); e == nil {
		t.Fatal("accepted concurrent writer")
	}
}
func TestBoundedRecordsAndFileFailureContinuesConsole(t *testing.T) {
	if len(bound([]byte(strings.Repeat("ü", 8000)))) > 4096 {
		t.Fatal("record unbounded")
	}
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "logs"), 0755)
	os.WriteFile(filepath.Join(root, "logs/error_log.txt"), []byte(fileHeader), 0400)
	c := configuration.DefaultLogging()
	c.Level = "INFO"
	c.ShowLog = true
	c.WriteLog = true
	var out bytes.Buffer
	r := Start(root, c, Options{Console: &out})
	Info("scene", "continued despite unwritable file")
	Info("scene", "another")
	r.Close()
	s := out.String()
	if strings.Count(s, "Log destination disabled") != 1 || !strings.Contains(s, "continued despite unwritable file") {
		t.Fatal(s)
	}
	root = t.TempDir()
	c.ShowLog = false
	r = Start(root, c, Options{})
	r.mu.Lock()
	r.file.file.Close()
	r.mu.Unlock()
	Info("test", "actual closed-fd error")
	Info("test", "no retry loop")
	notice := TakeNotice()
	if !strings.Contains(notice, "Log destination disabled") {
		t.Fatal(notice)
	}
	if TakeNotice() != "" {
		t.Fatal("repeated failure")
	}
	r.Close()
}
func TestBothDestinationsShareLevel(t *testing.T) {
	root := t.TempDir()
	c := configuration.DefaultLogging()
	c.ShowLog = true
	c.WriteLog = true
	c.Level = "ERROR"
	var out bytes.Buffer
	r := Start(root, c, Options{Console: &out})
	Log(slog.LevelWarn, "test", "no")
	Error("test", "yes")
	r.Close()
	b, _ := os.ReadFile(configuration.LogPath(root, c))
	if strings.Contains(out.String(), "msg=no") || strings.Contains(string(b), "msg=no") || !strings.Contains(out.String(), "msg=yes") || !strings.Contains(string(b), "msg=yes") {
		t.Fatal(out.String(), string(b))
	}
}

func TestActualTextRecordsRotateWithSmallCap(t *testing.T) {
	root := t.TempDir()
	c := configuration.DefaultLogging()
	c.Level = "INFO"
	c.WriteLog = true
	r := Start(root, c, Options{})
	r.file.cap = 900 // Exercise actual slog records, not a synthetic padded film.
	for i := 0; i < 30; i++ {
		Info("test", "bounded rollover", "ordinal", i)
	}
	r.Close()
	for _, path := range []string{configuration.LogPath(root, c), configuration.LogPath(root, c) + ".previous"} {
		b, e := os.ReadFile(path)
		if e != nil || len(b) > 900 || !bytes.HasPrefix(b, []byte(fileHeader)) {
			t.Fatal(path, len(b), e)
		}
		if !bytes.Contains(b, []byte("subsystem=test")) {
			t.Fatal(string(b))
		}
	}
}

func TestLegacyLineAndSingleVisibleCopy(t *testing.T) {
	var legacy, console bytes.Buffer
	ExistingLine(slog.LevelWarn, "audio", "fallback", nil, "audio: fallback", &legacy)
	if legacy.String() != "audio: fallback\n" {
		t.Fatal(legacy.String())
	}
	legacy.Reset()
	c := configuration.DefaultLogging()
	c.ShowLog = true
	r := Start(t.TempDir(), c, Options{Console: &console})
	ExistingLine(slog.LevelWarn, "audio", "fallback", nil, "audio: fallback", &legacy)
	r.Close()
	if legacy.Len() != 0 || strings.Count(console.String(), "msg=fallback") != 1 {
		t.Fatal(legacy.String(), console.String())
	}
}
