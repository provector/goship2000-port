package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/klauspost/compress/s2"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

// ImportFormat describes the framing used by an explicitly requested conversion.
// The game supplies only the current format when opening a recording.
type ImportFormat struct {
	Core, Published string
	Codec           string
	Rows            func([]byte, uint32) ([]Operation, error)
}
type ImportMetadata struct {
	Unscored *UnscoredDisposition
	Recovery *RecoveryReceipt
}

// ConvertContainer streams the authenticated source and verifies the temporary output before publication.
func ConvertContainer(src, dest string, format ImportFormat) error {
	orig, info, e := regularFilm(src)
	if e != nil {
		return e
	}
	defer orig.Close()
	f, pub, e := openFormat(orig, info.Size(), "", format)
	if e != nil {
		return e
	}
	var meta ImportMetadata
	if pub != nil {
		meta = ImportMetadata{pub.pub.Unscored, pub.pub.Recovery}
		if meta.Recovery != nil {
			h := sha256.New()
			h.Write([]byte(format.Core))
			if _, e = io.CopyN(h, io.NewSectionReader(orig, 12, pub.core-12), pub.core-12); e != nil {
				return e
			}
			if e = meta.Recovery.MatchesDigest(fmt.Sprintf("%x", h.Sum(nil)), f); e != nil {
				return e
			}
		}
	}
	for _, name := range []string{src + ".title.json", src + ".unscored.json", src + ".retained", filepath.Join(filepath.Dir(src), f.Header.ID+".recovery.json")} {
		if _, err := os.Lstat(name); !os.IsNotExist(err) {
			return fmt.Errorf("separate recording metadata needs explicit migration: %s", filepath.Base(name))
		}
	}
	tmp, e := os.CreateTemp(filepath.Dir(dest), ".codec-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, e = tmp.Write(privatePrefix); e != nil {
		return e
	}
	var idx bytes.Buffer
	header := f.Header
	header.Title = "Recovery identity"
	start, e := json.Marshal(recordedStart{header, f.Launch})
	if e != nil {
		return e
	}
	d, e := writeFrame(tmp, 1, 0, start)
	if e != nil {
		return e
	}
	binary.Write(&idx, order, d)
	var pending []byte
	var count uint32
	flush := func() error {
		if count == 0 {
			return nil
		}
		data := s2.Encode(nil, pending)
		at, e := tmp.Seek(0, io.SeekCurrent)
		if e != nil {
			return e
		}
		if at+int64(idx.Len()+len(data)+descriptorSize*2) > MaxBytes {
			return fmt.Errorf("converted recording exceeds limit")
		}
		d, e := writeCompressedFrame(tmp, 2, count, len(pending), data)
		if e != nil {
			return e
		}
		if e = binary.Write(&idx, order, d); e != nil {
			return e
		}
		count = 0
		pending = pending[:0]
		return nil
	}
	if f.OperationCount() > MaxOperations {
		return fmt.Errorf("too many recording operations")
	}
	var events uint64
	for i := 0; i < f.OperationCount(); i++ {
		o, e := f.OperationAt(i)
		if e != nil {
			return e
		}
		if e = validOperation(o); e != nil {
			return e
		}
		for _, ev := range o.Events {
			events++
			if ev.Index != events || ev.Kind < 1 || ev.Kind > 6 {
				return fmt.Errorf("operation %d: invalid event sequence", i)
			}
		}
		row, e := protoRow(o)
		if e != nil {
			return e
		}
		if len(row) > maxRow {
			return invalidBlock
		}
		if len(pending)+len(row) > blockTarget {
			if e = flush(); e != nil {
				return e
			}
		}
		pending = append(pending, row...)
		count++
	}
	if e = flush(); e != nil {
		return e
	}
	at, e := tmp.Seek(0, io.SeekCurrent)
	if e != nil {
		return e
	}
	m := manifest{protoS2, f.Header, f.End, at, idx.Len() / descriptorSize, BytesDigest(idx.Bytes())}
	if _, e = tmp.Write(idx.Bytes()); e != nil {
		return e
	}
	if e = writeManifest(tmp, m); e != nil {
		return e
	}
	if meta.Unscored != nil {
		u := *meta.Unscored
		u.ContentSHA256 = frameIdentity(m)
		meta.Unscored = &u
	}
	if meta.Recovery != nil {
		rec := *meta.Recovery
		if e = tmp.Sync(); e != nil {
			return e
		}
		coreSize, e := tmp.Seek(0, io.SeekCurrent)
		if e != nil {
			return e
		}
		rec.FilmSHA256, e = portableCoreDigest(tmp.Name(), coreSize)
		if e != nil {
			return e
		}
		rec.ContentSHA256 = frameIdentity(m)
		meta.Recovery = &rec
	}
	if e = appendPublication(tmp, m, publication{Title: f.Header.Title, Unscored: meta.Unscored, Recovery: meta.Recovery}, MaxBytes); e != nil {
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	out, e := OpenFile(tmp.Name())
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(f.Header, out.Header) || !reflect.DeepEqual(f.End, out.End) || !bytes.Equal(f.Launch, out.Launch) || f.OperationCount() != out.OperationCount() {
		return invalidBlock
	}
	for i := 0; i < f.OperationCount(); i++ {
		a, e := f.OperationAt(i)
		if e != nil {
			return e
		}
		b, e := out.OperationAt(i)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("converted operation %d differs", i)
		}
	}
	// Same-directory no-replace publication after closing, syncing and readback.
	if e = os.Link(tmp.Name(), dest); e != nil {
		return e
	}
	return syncFilmDirectory(filepath.Dir(dest))
}

func ImportID(path string, format ImportFormat) (string, error) {
	f, st, e := regularFilm(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	m, _, _, _, e := readContainerFormat(f, st.Size(), format)
	return m.Header.ID, e
}
