// G2R3 retains the G2R2 frame and directory offsets. Its immutable publication
// record follows the original ending. Two fixed-size title slots allow a torn
// rename to fall back to the previous generation without moving any frame.
package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const titleSlotSize = 96
const pubTailSize = 44 + 2*titleSlotSize
const maxPublicationBytes = MaxRecoveryBytes + 10*1024

var errLegacyRecording = errors.New("legacy recording requires manual conversion in SETUP")
var publicPrefix = []byte("GS2FILM\x00\x04\x00\x04\x00")
var privatePrefix = []byte("GS2FILM\x00\x04\x00\x01\x00")

func publishedPrefix(b []byte) bool { return bytes.Equal(b, publicPrefix) }
func currentFormat() ImportFormat {
	return ImportFormat{string(blockPrefix), string(publicPrefix), protoS2, decodeProtoRows}
}

type publication struct {
	Revision, Title string
	Unscored        *UnscoredDisposition `json:",omitempty"`
	Recovery        *RecoveryReceipt     `json:",omitempty"`
}
type portableInfo struct {
	pub        publication
	core       int64
	generation uint64
	slot       int
	binding    [32]byte
	title      string
}
type originalPrefixReader struct{ io.ReaderAt }

func (r originalPrefixReader) ReadAt(b []byte, off int64) (int, error) {
	n, e := r.ReaderAt.ReadAt(b, off)
	if off >= 0 && off < 12 {
		copy(b[:n], blockPrefix[off:])
	}
	return n, e
}
func encodeSlot(binding [32]byte, seq uint64, title string) []byte {
	b := make([]byte, titleSlotSize)
	order.PutUint64(b, seq)
	b[8] = byte(len(title))
	copy(b[9:], title)
	h := sha256.New()
	h.Write(binding[:])
	h.Write(b[:64])
	copy(b[64:], h.Sum(nil))
	return b
}
func readContainer(r io.ReaderAt, size int64) (manifest, int64, string, *portableInfo, error) {
	return readContainerFormat(r, size, currentFormat())
}
func readContainerFormat(r io.ReaderAt, size int64, format ImportFormat) (manifest, int64, string, *portableInfo, error) {
	var b [12]byte
	var zero manifest
	if _, e := r.ReadAt(b[:], 0); e != nil {
		return zero, 0, "", nil, e
	}
	if bytes.Equal(b[:], privatePrefix) {
		return zero, 0, "", nil, fmt.Errorf("private recording; no completed publication")
	}
	if string(b[:]) != format.Published {
		if string(b[:]) != format.Core {
			if bytes.Equal(b[:8], magic[:]) && (order.Uint16(b[8:10]) == 2 || order.Uint16(b[8:10]) == 3) {
				return zero, 0, "", nil, errLegacyRecording
			}
			return zero, 0, "", nil, fmt.Errorf("unsupported G2R storage version or encoding (header %x)", b[:])
		}
		m, at, rev, e := readManifest(originalPrefixReader{r}, size)
		if e == nil && m.Codec != format.Codec {
			e = invalidBlock
		}
		return m, at, rev, nil, e
	}
	if size < 12+pubTailSize || size > MaxBytes {
		return zero, 0, "", nil, invalidBlock
	}
	tail := make([]byte, pubTailSize)
	if _, e := r.ReadAt(tail, size-pubTailSize); e != nil {
		return zero, 0, "", nil, e
	}
	n := int64(order.Uint32(tail[8:12]))
	core := size - pubTailSize - n
	if string(tail[:8]) != "G2R3PUBL" || n < 1 || n > maxPublicationBytes || core < 12 {
		return zero, 0, "", nil, invalidBlock
	}
	raw := make([]byte, n)
	if _, e := r.ReadAt(raw, core); e != nil {
		return zero, 0, "", nil, e
	}
	p := &portableInfo{core: core, binding: sha256.Sum256(raw), slot: -1}
	if !bytes.Equal(p.binding[:], tail[12:44]) || strictJSON(raw, &p.pub) != nil {
		return zero, 0, "", nil, invalidBlock
	}
	m, at, rev, e := readManifest(originalPrefixReader{r}, core)
	if e != nil {
		return zero, 0, "", nil, e
	}
	if format.Codec != m.Codec {
		return zero, 0, "", nil, invalidBlock
	}
	if p.pub.Revision != rev || !m.Header.Complete || !ValidTitle(p.pub.Title) {
		return zero, 0, "", nil, invalidBlock
	}
	if u := p.pub.Unscored; u != nil && (u.Version != 1 || u.FilmID != m.Header.ID || u.ContentSHA256 != frameIdentity(m) || u.Reason == "" || len(u.Reason) > 4096) {
		return zero, 0, "", nil, invalidBlock
	}
	if rec := p.pub.Recovery; rec != nil && (rec.Validate() != nil || rec.FilmID != m.Header.ID || rec.ContentSHA256 != frameIdentity(m)) {
		return zero, 0, "", nil, invalidBlock
	}
	for i := 0; i < 2; i++ {
		s := tail[44+i*titleSlotSize : 44+(i+1)*titleSlotSize]
		seq := order.Uint64(s)
		l := int(s[8])
		if seq == 0 || l > 20 {
			continue
		}
		t := string(s[9 : 9+l])
		if ValidTitle(t) && bytes.Equal(s, encodeSlot(p.binding, seq, t)) && seq > p.generation {
			p.generation = seq
			p.slot = i
			p.title = t
		}
	}
	if p.slot < 0 {
		return zero, 0, "", nil, fmt.Errorf("no valid film title slot")
	}
	return m, at, rev, p, nil
}
func appendPublication(file *os.File, m manifest, p publication, limit int64) error {
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	p.Revision = BytesDigest(b)
	raw, e := json.Marshal(p)
	if e != nil {
		return e
	}
	at, e := file.Seek(0, io.SeekEnd)
	if e != nil {
		return e
	}
	if len(raw) > maxPublicationBytes || at+int64(len(raw))+pubTailSize > limit {
		return fmt.Errorf("portable metadata exceeds stored recording budget")
	}
	h := sha256.Sum256(raw)
	tail := make([]byte, pubTailSize)
	copy(tail, "G2R3PUBL")
	order.PutUint32(tail[8:], uint32(len(raw)))
	copy(tail[12:], h[:])
	copy(tail[44:], encodeSlot(h, 1, p.Title))
	if _, e = file.Write(raw); e == nil {
		_, e = file.Write(tail)
	}
	if e == nil {
		e = file.Sync()
	}
	if e != nil {
		return e
	}
	if _, e = file.WriteAt(publicPrefix, 0); e != nil {
		return e
	}
	return file.Sync()
}
func renameSlot(path string, p *portableInfo, title string) error {
	f, e := os.OpenFile(path, os.O_RDWR, 0)
	if e != nil {
		return e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return e
	}
	_, _, _, now, e := readContainer(f, st.Size())
	if e != nil {
		return e
	}
	if now == nil || now.binding != p.binding || now.generation != p.generation || p.generation == ^uint64(0) {
		return fmt.Errorf("film changed during rename")
	}
	_, e = f.WriteAt(encodeSlot(p.binding, p.generation+1, title), st.Size()-2*titleSlotSize+int64(1-p.slot)*titleSlotSize)
	if e != nil {
		return e
	}
	return f.Sync()
}
func portableCoreDigest(path string, core int64) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.NewSectionReader(originalPrefixReader{f}, 0, core))
	if e != nil || n != core {
		return "", invalidBlock
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func metadataAt(path string) (filmMetadata, error) {
	f, st, e := regularFilm(path)
	if e != nil {
		return filmMetadata{}, e
	}
	defer f.Close()
	m, e := baseMetadata(f, st)
	if e == nil && m.portable == nil {
		m.overlayTitle(path)
	}
	return m, e
}
func (s Store) matching(id string) ([]string, error) {
	entries, e := os.ReadDir(s.Root)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var paths []string
	var revision string
	for _, entry := range entries {
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".g2r") {
			continue
		}
		p := filepath.Join(s.Root, entry.Name())
		m, e := metadataAt(p)
		if e != nil {
			if entry.Name() == id+".g2r" {
				return nil, e
			}
			continue
		}
		if m.Header.ID != id {
			if entry.Name() == id+".g2r" {
				return nil, fmt.Errorf("occupied film name belongs to another identity")
			}
			continue
		}
		if len(paths) > 0 && revision != m.selection() {
			return nil, fmt.Errorf("conflicting recordings claim ID %s", id)
		}
		revision = m.selection()
		paths = append(paths, p)
	}
	return paths, nil
}
func (s Store) resolved(id string, mutate bool) (string, error) {
	canonical, e := s.path(id)
	if e != nil {
		return "", e
	}
	ps, e := s.matching(id)
	if e != nil {
		return "", e
	}
	if len(ps) == 0 {
		return canonical, nil
	}
	if mutate && len(ps) > 1 {
		return "", errors.New("multiple copies share this ID; keep one before editing")
	}
	return ps[0], nil
}
