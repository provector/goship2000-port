package configuration

import (
	"encoding/json"
	"gunship2000-assets/internal/userpack"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocationMigrationAndOwnership(t *testing.T) {
	install := t.TempDir()
	data := filepath.Join(t.TempDir(), "Independent user")
	other := t.TempDir()
	t.Setenv("GOSHIP2000_DATA_DIR", data)
	d, _, e := Initialize(filepath.Join(install, Filename))
	if e != nil {
		t.Fatal(e)
	}
	d.Config.Assets = "prepared"
	d.Config.Saves = "pilot files"
	d.Config.Original = "../original"
	if e = os.MkdirAll(Relative(install, d.Config.Original), 0700); e != nil {
		t.Fatal(e)
	}
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	rel := filepath.Join("pilot files", "replays", "test.g2r")
	src := filepath.Join(install, rel)
	if e = os.MkdirAll(filepath.Dir(src), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(src, []byte("opaque recording and recovery bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	hashes := map[string]string{}
	for _, n := range []string{Filename, rel} {
		h, e := userpack.HashFile(filepath.Join(install, n))
		if e != nil {
			t.Fatal(e)
		}
		hashes[n] = h
	}
	// Stop after a durable transfer intent and the first file. A different
	// installation cannot adopt a partially transferred package.
	if e = os.MkdirAll(data, 0700); e != nil {
		t.Fatal(e)
	}
	tr := transfer{Version: 1, Source: install, Files: hashes, Dirs: []string{"pilot files", filepath.Dir(rel)}}
	raw, _ := json.Marshal(tr)
	if e = safeRecord(filepath.Join(data, transferRecord), raw); e != nil {
		t.Fatal(e)
	}
	if e = copyVerified(filepath.Join(install, Filename), filepath.Join(data, Filename), hashes[Filename]); e != nil {
		t.Fatal(e)
	}
	if o, e := OpenLocationAt(other, "", "", false); e == nil {
		o.Close()
		t.Fatal("foreign partial transfer admitted")
	}
	o, e := OpenLocationAt(install, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { o.Close() })
	for n, h := range hashes {
		got, e := userpack.HashFile(filepath.Join(data, n))
		if e != nil || got != h {
			t.Fatal(n, got, e)
		}
		if _, e = os.Stat(filepath.Join(install, n)); !os.IsNotExist(e) {
			t.Fatal("duplicate remained", n, e)
		}
	}
	paths, e := LoadApplicationAt(install, data, "")
	if e != nil || paths.Original != Relative(install, "../original") || paths.Assets != filepath.Join(install, "prepared") || paths.Saves != filepath.Join(data, "pilot files") {
		t.Fatal(paths, e)
	}
	if second, e := OpenLocationAt(other, "", "", false); e == nil {
		second.Close()
		t.Fatal("two owners")
	} else if !strings.Contains(e.Error(), "already in use") {
		t.Fatal(e)
	}
	o.Close()
	if e = os.WriteFile(filepath.Join(other, "config.cfg"), []byte("unrelated original"), 0600); e != nil {
		t.Fatal(e)
	}
	next, e := OpenLocationAt(other, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	next.Close()
	if b, e := os.ReadFile(filepath.Join(other, "config.cfg")); e != nil || string(b) != "unrelated original" {
		t.Fatal("merged or retired conflicting source")
	}
	if e = os.WriteFile(filepath.Join(data, Filename), []byte("damaged"), 0600); e != nil {
		t.Fatal(e)
	}
	if next, e = OpenLocationAt(other, "", "", false); e == nil {
		next.Close()
		t.Fatal("corrupt active pack caused fallback")
	}
}
func TestPreferredFallback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux HOME resolution; Windows uses the OS Documents API")
	}
	home, install := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOSHIP2000_DATA_DIR", "")
	path := filepath.Join(home, ".goship2000")
	loc, e := OpenLocationAt(install, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	loc.Close()
	if loc.Data != path {
		t.Fatal(loc.Data)
	}
	if e = os.Remove(path); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("unusable"), 0600); e != nil {
		t.Fatal(e)
	}
	loc, e = OpenLocationAt(install, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	defer loc.Close()
	if loc.Data != install {
		t.Fatal(loc.Data)
	}
}

func TestExternalCareerKept(t *testing.T) {
	for _, absolute := range []bool{false, true} {
		base := t.TempDir()
		install := filepath.Join(base, "distribution")
		data := filepath.Join(base, "personal")
		os.Mkdir(install, 0700)
		d, _, e := Initialize(filepath.Join(install, Filename))
		if e != nil {
			t.Fatal(e)
		}
		location := "../custom"
		if absolute {
			location = filepath.Join(install, "saves")
		}
		d.Config.Saves = location
		if e = d.Save(); e != nil {
			t.Fatal(e)
		}
		store := Relative(install, location)
		os.MkdirAll(store, 0700)
		token := filepath.Join(store, "player0.g2pak")
		os.WriteFile(token, []byte("preserve"), 0600)
		t.Setenv("GOSHIP2000_DATA_DIR", data)
		o, e := OpenLocationAt(install, "", "", false)
		if e != nil {
			t.Fatal(e)
		}
		o.Close()
		if b, e := os.ReadFile(token); e != nil || string(b) != "preserve" {
			t.Fatal("moved external store", e)
		}
		other := t.TempDir()
		paths, e := LoadApplicationAt(other, data, "")
		if e != nil || paths.Saves != store {
			t.Fatal(paths.Saves, store, e)
		}
	}
}

func TestNestedInstallAndData(t *testing.T) {
	for _, inside := range []bool{false, true} {
		parent := t.TempDir()
		install, data := parent, filepath.Join(parent, "GoShip 2000")
		if inside {
			data, install = parent, filepath.Join(parent, "release")
		}
		if err := os.MkdirAll(install, 0700); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Initialize(filepath.Join(install, Filename)); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOSHIP2000_DATA_DIR", data)
		loc, err := OpenLocationAt(install, "", "", false)
		if err != nil {
			t.Fatal(err)
		}
		loc.Close()
		if _, err := os.Stat(filepath.Join(data, Filename)); err != nil {
			t.Fatal(err)
		}
	}
	install := t.TempDir()
	for _, dir := range []string{"saves", "assets", "original"} {
		data := filepath.Join(install, dir, "nested")
		t.Setenv("GOSHIP2000_DATA_DIR", data)
		if loc, err := OpenLocationAt(install, "", "", false); err == nil {
			loc.Close()
			t.Fatal("unsafe nested root", data)
		}
		if _, err := os.Stat(data); !os.IsNotExist(err) {
			t.Fatal("wrote rejected root", data, err)
		}
	}
}
