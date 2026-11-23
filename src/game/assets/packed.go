package assets

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

const packCatalogName = "packs.json"
const introChunkBytes = 4 * StartupPixels
const maxPackedResource = 96 << 20

var preparedPacks = [...]string{"core.g2pak", "audio.g2pak", "intro.g2pak"}

type leafProof struct {
	Hash string
	Size int
}
type packChunk struct {
	Size   int
	SHA256 string
}
type packedFile struct {
	Pack   int
	Codec  string
	Size   int
	Header []byte `json:",omitempty"` // Original WAV framing before the lossless PCM.
	Chunks []packChunk
	parts  []packPart
}
type packCatalog struct {
	Version int
	Digests [3]string
	Files   map[string]*packedFile
	Omitted map[string]leafProof
}
type packPart struct {
	at     int64
	stored int
}
type packedStore struct {
	catalogHash string
	root        string
	catalog     packCatalog
	handles     [3]*os.File
}

func (r *preparedReader) close() {
	if r.packs != nil {
		r.packs.close()
	}
}
func (p *packedStore) close() {
	for _, f := range p.handles {
		if f != nil {
			f.Close()
		}
	}
}
func safePackName(name string) bool {
	return fs.ValidPath(name) && name != "." && !strings.ContainsAny(name, ":\\\x00")
}
func chunkName(path, codec string, i int) string {
	switch codec {
	case "store":
		return path
	case "zstd":
		return path + ".zst"
	case "flac":
		return fmt.Sprintf("%s/%04d.flac", path, i)
	default:
		return fmt.Sprintf("%s/%04d.zst", path, i)
	}
}
func regularPack(path string, max int64) (*os.File, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > max {
		return nil, 0, fmt.Errorf("invalid pack type/size: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(opened, info) {
		f.Close()
		return nil, 0, fmt.Errorf("pack changed during open: %s", path)
	}
	return f, info.Size(), nil
}
func openPacks(root string) (store *packedStore, err error) {
	path := filepath.Join(root, packCatalogName)
	f, _, err := regularPack(path, 8<<20)
	if os.IsNotExist(err) {
		for _, name := range preparedPacks {
			if _, e := os.Lstat(filepath.Join(root, name)); e == nil {
				return nil, fmt.Errorf("missing %s beside %s", packCatalogName, name)
			} else if !os.IsNotExist(e) {
				return nil, e
			}
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8<<20+1))
	if err != nil {
		return nil, err
	}
	p := &packedStore{root: root, catalogHash: Hash(raw)}
	defer func() {
		if err != nil {
			p.close()
		}
	}()
	if err = DecodeObject(raw, &p.catalog); err != nil {
		return nil, fmt.Errorf("pack catalogue: %w", err)
	}
	if p.catalog.Version != 1 || len(p.catalog.Files) == 0 || len(p.catalog.Files)+len(p.catalog.Omitted) > 10000 {
		return nil, fmt.Errorf("unsupported or oversized pack catalogue")
	}
	entries := map[string]packPart{}
	for i, name := range preparedPacks {
		f, size, e := regularPack(filepath.Join(root, name), 256<<20)
		if e != nil {
			return nil, e
		}
		p.handles[i] = f
		// Our writers emit plain ZIP32 without comments. Bound the directory before archive/zip allocates.
		var end [22]byte
		if _, e = f.ReadAt(end[:], size-22); e != nil || string(end[:4]) != "PK\x05\x06" || binary.LittleEndian.Uint16(end[10:]) > 16384 || binary.LittleEndian.Uint32(end[12:]) > 8<<20 {
			return nil, fmt.Errorf("invalid ZIP directory in %s", name)
		}
		h := sha256.New()
		if _, e = io.CopyN(h, f, size); e != nil || fmt.Sprintf("%x", h.Sum(nil)) != p.catalog.Digests[i] {
			return nil, fmt.Errorf("physical pack digest differs: %s", name)
		}
		z, e := zip.NewReader(f, size)
		if e != nil {
			return nil, e
		}
		for _, entry := range z.File {
			key := name + "/" + entry.Name
			if !safePackName(entry.Name) || entry.Method != zip.Store || entry.UncompressedSize64 != entry.CompressedSize64 || entry.UncompressedSize64 > maxPackedResource || !entry.Mode().IsRegular() {
				return nil, fmt.Errorf("invalid STORE entry: %s", key)
			}
			if _, ok := entries[key]; ok {
				return nil, fmt.Errorf("duplicate pack entry %s", key)
			}
			offset, e := entry.DataOffset()
			if e != nil || offset < 0 || int64(entry.CompressedSize64) > size-offset {
				return nil, fmt.Errorf("invalid extent %s", key)
			}
			entries[key] = packPart{offset, int(entry.CompressedSize64)}
		}
	}
	for path, item := range p.catalog.Files {
		if !safePackName(path) || item == nil || item.Pack < 0 || item.Pack >= len(preparedPacks) || item.Size < 0 || item.Size > maxPackedResource || len(item.Chunks) == 0 || len(item.Chunks) > 1024 {
			return nil, fmt.Errorf("invalid packed resource %s", path)
		}
		switch item.Codec {
		case "store", "zstd":
			if len(item.Chunks) != 1 {
				return nil, fmt.Errorf("invalid single entry %s", path)
			}
		case "frames":
		case "flac":
			if len(item.Header) != 44 {
				return nil, fmt.Errorf("invalid WAV header %s", path)
			}
		default:
			return nil, fmt.Errorf("unknown asset codec %s", item.Codec)
		}
		if _, both := p.catalog.Omitted[path]; both || (item.Codec != "flac" && len(item.Header) != 0) {
			return nil, fmt.Errorf("ambiguous packed resource %s", path)
		}
		total := len(item.Header)
		for j, c := range item.Chunks {
			key := preparedPacks[item.Pack] + "/" + chunkName(path, item.Codec, j)
			part, ok := entries[key]
			if !ok || c.Size < 0 || c.Size > item.Size-total || len(c.SHA256) != 64 {
				return nil, fmt.Errorf("invalid or missing chunk %s", key)
			}
			switch item.Codec {
			case "frames":
				if c.Size != min(introChunkBytes, item.Size-total) || c.Size%StartupPixels != 0 {
					return nil, fmt.Errorf("invalid frame chunk %s", key)
				}
			case "flac":
				if c.Size != min(2*musicChunkFrames, item.Size-total) || c.Size%2 != 0 {
					return nil, fmt.Errorf("invalid PCM chunk %s", key)
				}
			}
			item.parts = append(item.parts, part)
			total += c.Size
			delete(entries, key)
		}
		if total != item.Size {
			return nil, fmt.Errorf("invalid decoded extent %s", path)
		}
	}
	if len(entries) != 0 {
		return nil, fmt.Errorf("unindexed prepared pack entries")
	}
	return p, nil
}

var packDecoder, packDecoderErr = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(maxPackedResource), zstd.WithDecodeAllCapLimit(true))

func (item *packedFile) chunk(f io.ReaderAt, i int) ([]byte, error) {
	part := item.parts[i]
	c := item.Chunks[i]
	b := make([]byte, part.stored)
	if _, err := f.ReadAt(b, part.at); err != nil {
		return nil, err
	}
	if Hash(b) != c.SHA256 {
		return nil, fmt.Errorf("stored chunk %d digest differs", i)
	}
	var err error
	switch item.Codec {
	case "frames", "zstd":
		if packDecoderErr != nil {
			return nil, packDecoderErr
		}
		b, err = packDecoder.DecodeAll(b, make([]byte, 0, c.Size))
	case "flac":
		b, err = decodePCMChunk(b, c.Size)
	}
	if err != nil {
		return nil, err
	}
	if len(b) != c.Size {
		return nil, fmt.Errorf("decoded chunk %d size differs", i)
	}
	return b, nil
}
func (p *packedStore) emit(path string, w io.Writer) error {
	item := p.catalog.Files[path]
	if item == nil {
		return fmt.Errorf("missing runtime asset %q", path)
	}
	if _, err := w.Write(item.Header); err != nil {
		return err
	}
	for i := range item.parts {
		b, e := item.chunk(p.handles[item.Pack], i)
		if e != nil {
			return fmt.Errorf("%s chunk %d: %w", path, i, e)
		}
		if _, e = w.Write(b); e != nil {
			return e
		}
	}
	return nil
}
func (p *packedStore) read(path string, limit int64) ([]byte, error) {
	item := p.catalog.Files[path]
	if item == nil || int64(item.Size) > limit {
		return nil, fmt.Errorf("missing or oversized packed resource %q", path)
	}
	if len(item.parts) == 1 && len(item.Header) == 0 {
		return item.chunk(p.handles[item.Pack], 0)
	}
	var b bytes.Buffer
	b.Grow(item.Size)
	if e := p.emit(path, &b); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func (p *packedStore) verifyMedia(path string, o Output) error {
	h := sha256.New()
	if err := p.emit(path, h); err != nil {
		return err
	}
	if p.catalog.Files[path].Size != o.Size || fmt.Sprintf("%x", h.Sum(nil)) != o.SHA256 {
		return fmt.Errorf("decoded media differs: %s", path)
	}
	return nil
}
