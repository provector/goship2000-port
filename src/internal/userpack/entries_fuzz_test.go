package userpack

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzStoreEntries(f *testing.F) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := zip.FileHeader{Name: "settings.json", Method: zip.Store}
	entry, e := w.CreateHeader(&h)
	if e != nil {
		f.Fatal(e)
	}
	if _, e = entry.Write([]byte("{}")); e != nil {
		f.Fatal(e)
	}
	if e = w.Close(); e != nil {
		f.Fatal(e)
	}
	f.Add(b.Bytes())
	f.Add(b.Bytes()[:30])
	f.Add([]byte{})
	path := filepath.Join(f.TempDir(), "input.g2pak")
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 64<<10 {
			return
		}
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
		a, e := Open(path)
		if e != nil {
			return
		}
		for name, v := range a {
			if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || v.Size > uint64(len(b)) {
				t.Fatalf("invalid entry %q", name)
			}
			raw, err := v.Read()
			if err == nil && len(raw) != int(v.Size) {
				t.Fatal("CRC-checked entry has incorrect size")
			}
		}
	})
}
