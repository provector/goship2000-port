package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type filmMetadata struct {
	portable           *portableInfo
	Header             Header
	End                End
	revision, identity string
	retained           bool
}

func regularFilm(p string) (*os.File, os.FileInfo, error) {
	st, e := os.Lstat(p)
	if e != nil {
		return nil, nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > MaxBytes {
		return nil, nil, fmt.Errorf("invalid managed film file")
	}
	file, e := os.Open(p)
	if e != nil {
		return nil, nil, e
	}
	info, e := file.Stat()
	if e != nil || !os.SameFile(st, info) || info.Size() > MaxBytes {
		file.Close()
		return nil, nil, fmt.Errorf("managed film changed during open")
	}
	return file, info, nil
}
func OpenFile(p string) (*Film, error) {
	file, info, err := regularFilm(p)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return openBlocks(file, info.Size(), p)
}
func frameIdentity(m manifest) string {
	m.Header.Title = "Recovery identity"
	b, _ := json.Marshal(m)
	return BytesDigest(b)
}
func baseMetadata(file *os.File, info os.FileInfo) (filmMetadata, error) {
	m, _, revision, p, err := readContainer(file, info.Size())
	if err != nil {
		return filmMetadata{}, err
	}
	out := filmMetadata{Header: m.Header, End: m.End, revision: revision, identity: frameIdentity(m), portable: p}
	if p != nil {
		out.Header.Title = p.title
	}
	return out, nil
}

type filmTitle struct{ Revision, Title string }

func smallFile(p string, limit int64) ([]byte, error) {
	f, st, e := regularFilm(p)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	if st.Size() > limit {
		return nil, fmt.Errorf("oversized film metadata")
	}
	return io.ReadAll(io.LimitReader(f, limit+1))
}
func (m *filmMetadata) overlayTitle(p string) {
	// Membership is separate from the optional display label. A lost label
	// cannot strand a previously retained Last Mission.
	b, e := smallFile(p+".title.json", 512)
	var t filmTitle
	if e == nil && strictJSON(b, &t) == nil && t.Revision == m.revision && ValidTitle(t.Title) && t.Title != "Last Mission" {
		m.Header.Title = t.Title
		m.retained = true
		return
	}
	marker, e := smallFile(p+".retained", 128)
	if e == nil && string(marker) == m.revision {
		m.retained = true
		last, _ := (Store{filepath.Dir(p)}).lastID()
		if m.Header.Title == "Last Mission" && last != m.Header.ID {
			m.Header.Title = "Retained Mission"
		}
	}

}
func (m filmMetadata) selection() string {
	key := m.revision
	if m.portable != nil {
		key += fmt.Sprintf("%x", m.portable.binding)
	}
	return BytesDigest([]byte(key + "\x00" + m.Header.Title))
}
func (s Store) metadata(id string) (filmMetadata, error) {
	p, e := s.resolved(id, false)
	if e != nil {
		return filmMetadata{}, e
	}
	return metadataAt(p)
}
func (s Store) selected(e Entry) (filmMetadata, error) {
	m, err := s.metadata(e.ID)
	if err == nil && e.Revision != "" && e.Revision != m.selection() {
		err = fmt.Errorf("selected film changed; reopen the library")
	}
	return m, err
}
func (s Store) loadSelected(entry Entry) (*Film, error) {
	p, e := s.resolved(entry.ID, false)
	if e != nil {
		return nil, e
	}
	file, info, e := regularFilm(p)
	if e != nil {
		return nil, e
	}
	defer file.Close()
	f, e := openBlocks(file, info.Size(), p)
	if e != nil {
		return nil, e
	}
	if f.Header.ID != entry.ID {
		return nil, invalidBlock
	}
	m, e := metadataAt(p)
	if e != nil {
		return nil, e
	}
	if entry.Revision != "" && entry.Revision != m.selection() {
		return nil, fmt.Errorf("selected film changed; reopen the library")
	}
	f.Header.Title = m.Header.Title
	f.blocks.managedFile = info
	return f, nil
}
func (s Store) publishSealed(f *Film, p string, pub publication) error {
	if err := f.Validate(); err != nil {
		return err
	}
	src := f.blocks
	expected := filepath.Join(filepath.Dir(p), ".film-"+f.Header.ID+".tmp.sealed")
	if src.path != expected {
		return fmt.Errorf("sealed film does not belong to this publication")
	}
	file, e := os.OpenFile(expected, os.O_RDWR, 0)
	if e != nil {
		return e
	}
	defer file.Close()
	st, e := file.Stat()
	if e != nil {
		return e
	}
	m, metaStart, _, e := readManifest(originalPrefixReader{file}, st.Size())
	if e != nil {
		return e
	}
	if m.IndexDigest != src.manifest.IndexDigest || m.Header.ID != f.Header.ID {
		return invalidBlock
	}
	// Recheck stored frame bytes and the directory before moving the only rolling
	// pointer. No operation is parsed or rewritten during this verification.
	hash := sha256.New()
	buf := make([]byte, 32<<10)
	at := int64(len(blockPrefix))
	for i := 0; i < m.Blocks; i++ {
		d, err := readDescriptor(file, at)
		if err != nil || !d.valid(i == 0) || at+descriptorSize+int64(d.Stored) > m.IndexOffset {
			return invalidBlock
		}
		expected, err := readDescriptor(file, m.IndexOffset+int64(i)*descriptorSize)
		if err != nil || d != expected {
			return invalidBlock
		}
		hash.Reset()
		n, err := io.CopyBuffer(hash, io.NewSectionReader(file, at+descriptorSize, int64(d.Stored)), buf)
		if err != nil || n != int64(d.Stored) || !bytes.Equal(hash.Sum(nil), d.Digest[:]) {
			return invalidBlock
		}
		at += descriptorSize + int64(d.Stored)
	}
	if at != m.IndexOffset {
		return invalidBlock
	}
	hash.Reset()
	n, err := io.Copy(hash, io.NewSectionReader(file, m.IndexOffset, int64(m.Blocks)*descriptorSize))
	if err != nil || n != int64(m.Blocks)*descriptorSize || fmt.Sprintf("%x", hash.Sum(nil)) != m.IndexDigest {
		return invalidBlock
	}
	activated := false
	var savedFooter [footerSize]byte
	if _, e = file.ReadAt(savedFooter[:], st.Size()-footerSize); e != nil {
		return e
	}
	defer func() {
		if activated {
			return
		}
		// Best effort only after a failed write; an interruption at any stage
		// retains the checked operation prefix under its private working name.
		restore, err := os.OpenFile(expected, os.O_RDWR, 0)
		if err != nil {
			return
		}
		defer restore.Close()
		if _, err = restore.WriteAt(privatePrefix, 0); err != nil {
			return
		}
		if err = restore.Truncate(st.Size()); err != nil {
			return
		}
		if _, err = restore.WriteAt(savedFooter[:], st.Size()-footerSize); err == nil {
			_ = restore.Sync()
		}
	}()
	updated := m
	updated.Header = f.Header
	updated.End = f.End
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(updated)
	if !bytes.Equal(a, b) {
		if st.Size()+int64(len(b))+footerSize > src.limit {
			return fmt.Errorf("film publication metadata exceeds %d-byte storage limit", src.limit)
		}
		if metaStart, e = file.Seek(0, io.SeekEnd); e != nil {
			return e
		}
		if e = writeManifest(file, updated); e == nil {
			e = file.Sync()
		}
		if e != nil {
			_ = file.Truncate(st.Size())
			return e
		}
	}
	// A cut at the former private seal must never admit a preliminary score
	// as the public ending. Keep the earlier manifest for explicit inspection.
	if metaStart > m.IndexOffset+int64(m.Blocks)*descriptorSize {
		if _, e = file.WriteAt([]byte("G2R2SEAL"), metaStart-footerSize); e != nil {
			return e
		}
		if e = file.Sync(); e != nil {
			return e
		}
	}
	if e = appendPublication(file, updated, pub, src.limit); e != nil {
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	if e = moveFilmNew(expected, p); e != nil {
		return e
	}
	activated = true
	src.path = p
	src.manifest = updated
	src.revision = BytesDigest(b)
	return syncFilmDirectory(filepath.Dir(p))
}
