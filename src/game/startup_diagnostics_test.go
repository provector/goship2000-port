package main

import (
	"errors"
	assetdata "gunship2000-assets/game/assets"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupRetainsUnsupportedProfileCause(t *testing.T) {
	p, s, _ := fakeStartup(t)
	if _, e := ensureOfficeAssets(p, false, io.Discard, s); e != nil {
		t.Fatal(e)
	}
	marker := filepath.Join(p.Assets, "protected-profile")
	if e := os.WriteFile(marker, []byte("prior complete tree"), 0600); e != nil {
		t.Fatal(e)
	}
	s.ready = func(*assetdata.OfficeAssets) error { return errors.New("required current office metadata missing") }
	prepare := s.prepare
	s.prepare = func(a, b string) (*assetdata.Manifest, error) {
		m, e := prepare(a, b)
		if e == nil {
			m.UnsupportedVersionOperations = []assetdata.UnsupportedVersionOperation{{Operation: "office metadata", Reason: "GS.GS2 unsupported identity supplied-sha256", Profile: "supported source fixture"}}
		}
		return m, e
	}
	_, e := ensureOfficeAssets(p, false, io.Discard, s)
	if e == nil || !strings.Contains(e.Error(), "unsupported source profile: office metadata: GS.GS2 unsupported identity supplied-sha256") || !strings.Contains(e.Error(), "previous assets unchanged") {
		t.Fatal(e)
	}
	b, e := os.ReadFile(marker)
	if e != nil || string(b) != "prior complete tree" {
		t.Fatal("predecessor changed", e)
	}
}
