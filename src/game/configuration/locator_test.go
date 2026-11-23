package configuration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gunship2000-assets/internal/userpack"
)

func TestSwitchLocation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("GOSHIP2000_DATA_DIR", "")
	src := t.TempDir()
	install := t.TempDir()
	// On Windows the Documents API is independent of HOME. Select only private test data.
	locPath, e := locatorPath()
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS == "windows" && !Within(locPath, home) {
		t.Fatal(locPath)
	}
	if e = os.MkdirAll(filepath.Dir(locPath), 0700); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(locationChoice{1, src})
	if e = safeRecord(locPath, b); e != nil {
		t.Fatal(e)
	}
	_, _, e = InitializeAt(filepath.Join(src, Filename), install)
	if e != nil {
		t.Fatal(e)
	}
	loc, e := OpenLocationAt(install, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	defer loc.Close()
	film := filepath.Join("saves", "replays", "a.g2r")
	if e = os.MkdirAll(filepath.Dir(filepath.Join(src, film)), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(src, film), []byte("preserve this payload"), 0600); e != nil {
		t.Fatal(e)
	}
	noSave := func() error { return nil }
	deny := func(string, string) error { return errors.New("incomplete") }
	dst := t.TempDir()
	// Stop after verification and before publishing the locator.
	if e = transferUserStore(src, dst, install, true); e != nil {
		t.Fatal(e)
	}
	// Simulate termination midway through streaming a recording.
	if e = os.Remove(filepath.Join(dst, film)); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dst, switchScratch), []byte("partial"), 0600); e != nil {
		t.Fatal(e)
	}
	if p, _, _ := locatedData(); p != src {
		t.Fatal(p)
	}
	if e = loc.SwitchData(dst, false, noSave, deny); e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{Filename, film} {
		h, _ := userpack.HashFile(filepath.Join(src, n))
		v, e := userpack.HashFile(filepath.Join(dst, n))
		if e != nil || h != v {
			t.Fatal(n, e)
		}
	}
	if p, _, _ := locatedData(); p != dst {
		t.Fatal(p)
	}
	another := t.TempDir()
	if v, e := OpenLocationAt(another, "", "", false); e == nil {
		v.Close()
		t.Fatal("shared locator was not owned")
	}
	loc.Close()
	l, e := OpenLocationAt(another, "", "", false)
	if e != nil || l.Data != dst {
		t.Fatal(l, e)
	}
	defer l.Close()
	if e = l.SwitchData(src, false, noSave, deny); e == nil {
		t.Fatal("invalid roster admitted")
	}
	valid := func(string, string) error { return nil }
	if e = l.SwitchData(src, false, noSave, valid); !errors.Is(e, ErrExistingLocation) {
		t.Fatal(e)
	}
	if e = l.SwitchData(src, true, noSave, valid); e != nil {
		t.Fatal(e)
	}
	l.Close()
	if e = os.Rename(src, src+".offline"); e != nil {
		t.Fatal(e)
	}
	defer os.Rename(src+".offline", src)
	if l, e = OpenLocationAt(another, "", "", false); e == nil {
		l.Close()
		t.Fatal("offline explicit choice reseeded")
	}
}
