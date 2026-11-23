package main

import (
	"bytes"
	"gunship2000-assets/game/replay"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilmRecoveryCLIOutputGuards(t *testing.T) {
	if runFilms([]string{"-help"}) != 0 {
		t.Fatal("film help failed")
	}
	for _, a := range [][]string{nil, {"recover"}, {"unknown"}, {"verify-recovery", "-input", "missing"}} {
		if runFilms(a) == 0 {
			t.Fatal("incomplete/invalid command accepted")
		}
	}
	dir := t.TempDir()
	film := filepath.Join(dir, "copy.g2r")
	receipt := filepath.Join(dir, "receipt.json")
	original := []byte("preserve this explicit input")
	if e := os.WriteFile(film, original, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(receipt, []byte("existing receipt"), 0600); e != nil {
		t.Fatal(e)
	}
	if runFilms([]string{"recover", "-input", film, "-receipt", receipt, "-assets", dir}) == 0 {
		t.Fatal("overwrote receipt")
	}
	if runFilms([]string{"recover", "-input", film, "-receipt", film, "-assets", dir}) == 0 {
		t.Fatal("film replacement accepted")
	}
	b, _ := os.ReadFile(film)
	if !bytes.Equal(b, original) {
		t.Fatal("input bytes modified")
	}
	b, _ = os.ReadFile(receipt)
	if string(b) != "existing receipt" {
		t.Fatal("existing receipt changed")
	}
}

func TestFilmIdentifyArbitraryNameReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "full-new.g2r")
	groups := map[string]string{}
	for _, k := range []string{"flight", "mission", "combat", "navigation", "environment", "reference", "pending-hit", "palette", "visual"} {
		groups[k] = strings.Repeat("a", 64)
	}
	// Structurally valid container only: identify does not claim initialization
	// completeness, resources available, deterministic playback or career facts.
	film := &replay.Film{Header: replay.Header{Version: replay.Version, ID: strings.Repeat("b", 32), Title: "Arbitrary filename", Simulation: "native-non-ai-stepO-v1", InputSchema: "logical-operations-v1", InitializationSchema: "accepted-native-launch-v1", Resources: strings.Repeat("c", 64), Complete: true}, Launch: []byte(`{}`), Operations: []replay.Operation{{Kind: "end", Payload: []byte(`{"Reason":"test"}`), Checkpoint: groups}}, End: replay.End{Checkpoint: groups}}
	data, err := replay.Marshal(film)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = wr
	code := runFilms([]string{"identify", "-input", path})
	wr.Close()
	os.Stdout = old
	output, err := io.ReadAll(rd)
	rd.Close()
	if err != nil || code != 0 || string(output) != film.Header.ID+"\n" {
		t.Fatalf("identify output %q status%d: %v", output, code, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, data) {
		t.Fatal("identify modified film")
	}
	for _, bad := range [][]byte{[]byte("not a native film"), data[:len(data)-1]} {
		p := filepath.Join(dir, "broken.g2r")
		if err = os.WriteFile(p, bad, 0600); err != nil {
			t.Fatal(err)
		}
		if runFilms([]string{"identify", "-input", p}) == 0 {
			t.Fatal("invalid/truncated film identified")
		}
	}
	for _, args := range [][]string{{"identify"}, {"identify", "-input", filepath.Join(dir, "missing.g2r")}, {"identify", "-input", path, "unexpected"}} {
		if runFilms(args) == 0 {
			t.Fatal("missing/invalid identify arguments accepted")
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "full-new.g2r" && e.Name() != "broken.g2r" {
			t.Fatal("identify wrote unexpected file", e.Name())
		}
	}
}
