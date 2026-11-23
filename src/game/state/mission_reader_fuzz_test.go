package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func FuzzEditorDocument(f *testing.F) {
	b, e := os.ReadFile(filepath.Join("..", "..", "..", "test-data", "regression-missions", "End and Recovery.g2m"))
	if e != nil {
		f.Fatal(e)
	}
	f.Add(b)
	f.Add(b[:len(b)/2])
	f.Add([]byte("{}"))
	path := filepath.Join(f.TempDir(), "scratch.g2m")
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 96<<10 {
			return
		}
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
		d, _, e := readMissionDocument(path)
		if e != nil {
			return
		}
		var header struct{ ID, Title string }
		if e = json.Unmarshal(b, &header); e != nil || d.ID != header.ID || d.Title != header.Title || d.ID == "" {
			t.Fatal("reader changed identity or title", e)
		}
	})
}
