package menus

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScreenshotNamesNeverReplaceEarlierCaptures(t *testing.T) {
	saves := filepath.Join(t.TempDir(), "Łódź saves")
	at := time.Date(2027, 12, 12, 21, 4, 36, 0, time.FixedZone("local", 3600))
	for i, name := range []string{"12 Dec 2027 - 21-04-36.png", "12 Dec 2027 - 21-04-36-001.png", "12 Dec 2027 - 21-04-36-002.png"} {
		path, err := writeScreenshot(saves, at, func(w io.Writer) error { _, err := w.Write([]byte{byte(i)}); return err })
		if err != nil || path != filepath.Join(filepath.Dir(saves), "screenshots", name) {
			t.Fatal(path, err)
		}
	}
	first, err := os.ReadFile(filepath.Join(filepath.Dir(saves), "screenshots", "12 Dec 2027 - 21-04-36.png"))
	if err != nil || len(first) != 1 || first[0] != 0 {
		t.Fatal("original screenshot changed", first, err)
	}
}
func TestScreenshotUnavailableDirectory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "saves")
	dir := filepath.Join(filepath.Dir(p), "screenshots")
	if err := os.WriteFile(dir, []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := writeScreenshot(p, time.Now(), nil)
	if err == nil || got != dir {
		t.Fatal(got, err)
	}
}
