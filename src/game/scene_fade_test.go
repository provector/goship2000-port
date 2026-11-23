package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFadeConfiguration(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{}`), 0600)
	p, err := loadApplication(dir, "")
	if err != nil || p.Config.FadeMilliseconds != 150 {
		t.Fatal(p, err)
	}
	for _, value := range []string{"0", "2200", "-1"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.cfg"), []byte(`{"fade_milliseconds":`+value+`}`), 0600); err != nil {
			t.Fatal(err)
		}
		_, err = loadApplication(dir, "")
		if (err == nil) != (value == "0") {
			t.Fatal(value, err)
		}
	}
}
