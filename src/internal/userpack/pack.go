// Package userpack handles STORE-only player-owned ZIPs. The external owner of
// each document supplies the transaction/lease and schema validation.
package userpack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sync"
)

var configWrites sync.Mutex

const ConfigName = "config.g2pak"
const MaxEntry = 4 << 20 // Existing maximum size of an individual career document.
type Entry struct {
	Path   string
	Offset int64
	Size   uint64
	CRC    uint32
	Data   []byte
}
type Entries map[string]Entry

func Value(b []byte) Entry { return Entry{Data: b} }
func validName(n string) bool {
	return fs.ValidPath(n) && n != "." && !strings.ContainsAny(n, "\\:\x00")
}
func ordinary(path string) (os.FileInfo, error) {
	i, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() {
		return nil, fmt.Errorf("expected regular file: %s", path)
	}
	return i, nil
}
func Open(path string) (Entries, error) {
	i, e := ordinary(path)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	z, e := zip.NewReader(f, i.Size())
	if e != nil {
		return nil, fmt.Errorf("%s: %w", path, e)
	}
	out := Entries{}
	for _, v := range z.File {
		if !validName(v.Name) || !v.Mode().IsRegular() || v.Method != zip.Store || v.UncompressedSize64 > MaxEntry || v.UncompressedSize64 != v.CompressedSize64 {
			return nil, fmt.Errorf("%s: invalid STORE entry %q", path, v.Name)
		}
		if _, ok := out[v.Name]; ok {
			return nil, fmt.Errorf("duplicate entry %s", v.Name)
		}
		at, e := v.DataOffset()
		if e != nil || at < 0 || int64(v.UncompressedSize64) > i.Size()-at {
			return nil, fmt.Errorf("invalid extent %s", v.Name)
		}
		out[v.Name] = Entry{Path: path, Offset: at, Size: v.UncompressedSize64, CRC: v.CRC32}
	}
	return out, nil
}
func (v Entry) Read() ([]byte, error) {
	if v.Path == "" {
		return v.Data, nil
	}
	f, e := os.Open(v.Path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b := make([]byte, int(v.Size))
	if len(b) > 0 {
		if _, e = f.ReadAt(b, v.Offset); e != nil {
			return nil, e
		}
	}
	if crc32.ChecksumIEEE(b) != v.CRC {
		return nil, fmt.Errorf("STORE checksum differs in %s", v.Path)
	}
	return b, nil
}
func (a Entries) Get(n string) ([]byte, error) {
	v, ok := a[n]
	if !ok {
		return nil, os.ErrNotExist
	}
	return v.Read()
}
func Stage(dir string, a Entries) (path string, err error) {
	f, e := os.CreateTemp(dir, ".g2pak-stage-")
	if e != nil {
		return "", e
	}
	p := f.Name()
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(p)
		}
	}()
	names := make([]string, 0, len(a))
	for n := range a {
		names = append(names, n)
	}
	sort.Strings(names)
	z := zip.NewWriter(f)
	for _, n := range names {
		if !validName(n) {
			return "", fmt.Errorf("invalid entry %q", n)
		}
		b, e := a.Get(n)
		if e != nil {
			return "", e
		}
		if len(b) > MaxEntry {
			return "", fmt.Errorf("oversized entry %q", n)
		}
		w, e := z.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Store})
		if e != nil {
			return "", e
		}
		if _, e = w.Write(b); e != nil {
			return "", e
		}
	}
	if e = z.Close(); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	check, e := Open(p)
	if e != nil {
		return "", e
	}
	if len(check) != len(a) {
		return "", fmt.Errorf("staged pack entry count differs")
	}
	for n := range check {
		if _, e = check.Get(n); e != nil {
			return "", e
		}
	}
	return p, nil
}
func HashFile(path string) (string, error) {
	if _, e := ordinary(path); e != nil {
		return "", e
	}
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func Locked(path string, fn func() error) error {
	configWrites.Lock()
	defer configWrites.Unlock()
	return fn()
}
func ReadConfig(dir, name string) ([]byte, error) {
	a, e := Open(filepath.Join(dir, ConfigName))
	if e != nil {
		return nil, e
	}
	h, e := a.Get("format.json")
	if e != nil {
		return nil, fmt.Errorf("missing configuration pack format: %w", e)
	}
	var header struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(h, &header) != nil || header.Version != 1 {
		return nil, fmt.Errorf("unsupported configuration pack version")
	}
	return a.Get(name)
}

type Change struct {
	Name          string
	Before, After []byte
}

// PatchLocked compares only the entries owned by this caller, retaining other writers' data.
func PatchLocked(path string, cs []Change) error {
	st, e := ordinary(path)
	if e != nil {
		return e
	}
	if st.Mode().Perm()&0222 == 0 {
		return fmt.Errorf("read-only pack: %s", path)
	}
	fingerprint, e := HashFile(path)
	if e != nil {
		return e
	}
	a, e := Open(path)
	if e != nil {
		return e
	}
	dirty := false
	for _, c := range cs {
		b, e := a.Get(c.Name)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if !bytes.Equal(b, c.Before) || (b == nil) != (c.Before == nil) {
			return fmt.Errorf("%s changed externally; reopen before saving", c.Name)
		}
		if bytes.Equal(b, c.After) {
			continue
		}
		dirty = true
		if c.After == nil {
			delete(a, c.Name)
		} else {
			a[c.Name] = Value(c.After)
		}
	}
	if !dirty {
		return nil
	}
	stage, e := Stage(filepath.Dir(path), a)
	if e != nil {
		return e
	}
	defer os.Remove(stage)
	latest, e := HashFile(path)
	if e != nil {
		return e
	}
	if latest != fingerprint {
		return fmt.Errorf("pack changed while saving: %s", path)
	}
	return Replace(stage, path)
}
