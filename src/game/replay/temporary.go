package replay

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/klauspost/compress/s2"
	"io"
	"os"
	"path/filepath"

	"gunship2000-assets/game/configuration"
)

// RecordingIOError carries the reached I/O phase without losing its cause.
type RecordingIOError struct {
	Phase string
	Err   error
}

func (e *RecordingIOError) Error() string { return "film " + e.Phase + ": " + e.Err.Error() }
func (e *RecordingIOError) Unwrap() error { return e.Err }
func RecordingFailurePhase(err error, fallback string) string {
	var e *RecordingIOError
	if errors.As(err, &e) {
		return e.Phase
	}
	return fallback
}

// EndReserve bounds the production ending/summary and final framing admitted
// before accepting operations. It is not padding in a recorded film.
const EndReserve = MaxEndBytes

// A private seal and an amended public score can each require one footer.
// Allow the bounded unscored disposition or rolling pointer as well.
const sealReserve = maxPublicationBytes + pubTailSize + 2*(MaxHeaderBytes+MaxEndBytes+1024+footerSize) + 8192

// RecordingFailure is the first irreversible recorder-only failure. It has no
// career journal semantics and never authorizes an incomplete film for playback.
type RecordingFailure struct {
	Version              int
	FilmID, Phase, Cause string
	SessionID            string `json:",omitempty"`
	Operation, Step      uint64
	Bytes, Limit         int64
	Count                int
}

type Temporary struct {
	writeErr     error
	blocks       int
	pending      bytes.Buffer
	batch        uint32
	events       uint64
	sessionID    string
	file         *os.File
	buffer       *bufio.Writer
	path         string
	bytes, limit int64
	count        int
	id           string
	failure      *RecordingFailure
}

func (s Store) BeginTemporary(f *Film) (*Temporary, error) {
	return s.BeginTemporaryWithLimit(f, 0)
}
func (s Store) BeginTemporaryWithLimit(f *Film, limit configuration.RecordingLimit) (*Temporary, error) {
	if f == nil || len(f.Launch) == 0 || len(f.Launch) > 8<<20 {
		return nil, fmt.Errorf("recording launch must be present and at most8MiB")
	}
	limit = limit.Effective()
	if err := limit.Validate(); err != nil {
		return nil, err
	}
	p, e := s.path(f.Header.ID)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return nil, e
	}
	published, e := s.resolved(f.Header.ID, false)
	if e != nil {
		return nil, e
	}
	p = filepath.Join(filepath.Dir(p), ".film-"+f.Header.ID+".tmp")
	file, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	// Acquiring the temporary name serializes recorders with this ID. Check
	// the next two ownership stages after acquisition, in publication order.
	for _, occupied := range []string{p + ".sealed", published} {
		if _, err := os.Lstat(occupied); !os.IsNotExist(err) {
			file.Close()
			_ = os.Remove(p)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("recording identity already owns a sealed or published film")
		}
	}
	t := &Temporary{file: file, buffer: bufio.NewWriterSize(file, 65536), path: p, id: f.Header.ID, limit: int64(limit)}
	start, e := json.Marshal(recordedStart{f.Header, f.Launch})
	if e == nil {
		h, _ := json.Marshal(f.Header)
		if len(h) > MaxHeaderBytes {
			e = fmt.Errorf("film header exceeds %d bytes", MaxHeaderBytes)
		}
	}
	if e == nil {
		e = encoderErr
	}
	if e == nil {
		data := encoder.EncodeAll(start, nil)
		frameBytes := int64(len(blockPrefix) + descriptorSize + len(data))
		if frameBytes+descriptorSize+sealReserve > t.limit {
			e = fmt.Errorf("compressed launch and finalisation reserve exceed %d-byte recording limit", t.limit)
		} else {
			_, e = t.buffer.Write(privatePrefix)
			if e == nil {
				_, e = writeCompressedFrame(t.buffer, 1, 0, len(start), data)
			}
			if e == nil {
				e = t.buffer.Flush()
			}
			if e == nil {
				t.blocks = 1
				t.bytes = frameBytes
			}
		}
	}
	if e != nil {
		t.RecordFailure("begin", 0, 0, e)
		t.Close()
		return nil, e
	}
	return t, nil
}
func (t *Temporary) SetSessionID(id string) {
	if t != nil && (id == "" || ValidID(id)) {
		t.sessionID = id
	}
}
func (t *Temporary) Bytes() int64 {
	if t == nil {
		return 0
	}
	return t.bytes
}
func (t *Temporary) Limit() int64 {
	if t == nil {
		return 0
	}
	return t.limit
}
func (t *Temporary) Failure() *RecordingFailure {
	if t == nil {
		return nil
	}
	return t.failure
}
func (t *Temporary) RecordFailure(phase string, operation, step uint64, err error) {
	if t == nil || t.failure != nil || err == nil {
		return
	}
	t.failure = &RecordingFailure{Version: 1, FilmID: t.id, SessionID: t.sessionID, Phase: RecordingFailurePhase(err, phase), Cause: err.Error(), Operation: operation, Step: step, Bytes: t.bytes, Limit: t.limit, Count: t.count}
	b, _ := json.MarshalIndent(t.failure, "", "  ")
	// A failed diagnostics write must not replace the actual recording cause.
	f, e := os.CreateTemp(filepath.Dir(t.path), ".film-failure-*")
	if e != nil {
		return
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(append(b, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		_ = os.Rename(name, t.path+".failure.json")
	}
}
func (t *Temporary) Count() int { return t.count }
func (t *Temporary) flushBatch() error {
	if t.batch == 0 {
		return nil
	}
	d, e := writeFrame(t.buffer, 2, t.batch, t.pending.Bytes())
	if e != nil {
		t.writeErr = e
		return e
	}
	if e = t.buffer.Flush(); e != nil {
		t.writeErr = e
		return e
	}
	t.blocks++
	t.bytes += descriptorSize + int64(d.Stored)
	t.pending.Reset()
	t.batch = 0
	return nil
}
func (t *Temporary) Append(o Operation) error {
	if t == nil || t.file == nil {
		return fmt.Errorf("film spool closed")
	}
	if t.failure != nil {
		return fmt.Errorf("film spool failed: %s", t.failure.Cause)
	}
	if e := validOperation(o); e != nil {
		return e
	}
	for i, ev := range o.Events {
		if ev.Index != t.events+1+uint64(i) || ev.Kind < 1 || ev.Kind > 6 {
			return fmt.Errorf("invalid film event sequence")
		}
	}
	b, e := protoRow(o)
	if e != nil {
		return e
	}
	if len(b) > maxRow {
		return fmt.Errorf("encoded operation exceeds %d bytes", maxRow)
	}
	if t.count >= MaxOperations {
		return fmt.Errorf("film operation capacity reached")
	}
	n := len(b)
	if t.batch > 0 && t.pending.Len()+n > blockTarget {
		if e = t.flushBatch(); e != nil {
			return e
		}
	}
	old := t.pending.Len()
	t.pending.Write(b)

	// Fast admission for normal blocks. Close to the limit, measure the actual
	// compressed candidate before accepting this operation into the private prefix.
	room := t.limit - t.bytes - int64(t.blocks+1)*descriptorSize - descriptorSize - sealReserve
	if int64(s2.MaxEncodedLen(t.pending.Len())) > room {
		if int64(len(s2.Encode(nil, t.pending.Bytes()))) > room {
			t.pending.Truncate(old)
			return fmt.Errorf("compressed film reached %d-byte limit including directory and finalisation reserve", t.limit)
		}
	}
	t.batch++
	t.count++
	t.events += uint64(len(o.Events))

	return nil
}
func (t *Temporary) Seal(f *Film) error {
	if t == nil || t.file == nil {
		return fmt.Errorf("film spool closed")
	}
	if t.failure != nil {
		return fmt.Errorf("film spool failed: %s", t.failure.Cause)
	}
	if len(f.Operations) > 0 && len(f.Operations) != t.count {
		return fmt.Errorf("sealed operation count differs from spool")
	}
	if err := f.Validate(); err != nil {
		return err
	}
	if e := t.flushBatch(); e != nil {
		return &RecordingIOError{"flush", e}
	}
	if e := t.buffer.Flush(); e != nil {
		return &RecordingIOError{"flush", e}
	}
	if e := t.file.Sync(); e != nil {
		return &RecordingIOError{"prefix-sync", e}
	}
	at, e := t.file.Seek(0, 1)
	if e != nil {
		return e
	}
	// Copy the existing frame headers into the directory. Only one descriptor
	// and a bounded write buffer are live, even for a very compressible sortie.
	hash := sha256.New()
	dir := io.MultiWriter(t.buffer, hash)
	pos := int64(len(blockPrefix))
	var header [descriptorSize]byte
	for i := 0; i < t.blocks && e == nil; i++ {
		if _, e = t.file.ReadAt(header[:], pos); e != nil {
			break
		}
		d := parseDescriptor(header[:])
		if !d.valid(i == 0) {
			e = invalidBlock
			break
		}
		pos += descriptorSize + int64(d.Stored)
		_, e = dir.Write(header[:])
	}
	if e == nil && pos != at {
		e = invalidBlock
	}
	if e == nil {
		e = t.buffer.Flush()
	}
	m := manifest{protoS2, f.Header, f.End, at, t.blocks, fmt.Sprintf("%x", hash.Sum(nil))}
	if e == nil {
		e = writeManifest(t.file, m)
	}
	if e == nil {
		e = t.file.Sync()
	}
	if e != nil {
		_ = t.file.Truncate(at)
		return &RecordingIOError{"seal-staging", e}
	}
	info, e := t.file.Stat()
	if e != nil {
		return e
	}
	if info.Size() > t.limit {
		return fmt.Errorf("sealed film exceeds selected storage limit")
	}
	sealed, e := openBlocks(originalPrefixReader{t.file}, info.Size(), t.path+".sealed")
	if e != nil {
		return e
	}
	if e = t.file.Close(); e != nil {
		return e
	}
	t.file = nil
	if e = os.Rename(t.path, t.path+".sealed"); e != nil {
		return &RecordingIOError{"seal-rename", e}
	}
	if e = syncFilmDirectory(filepath.Dir(t.path)); e != nil {
		return &RecordingIOError{"seal-directory-sync", e}
	}
	sealed.blocks.limit = t.limit
	f.blocks = sealed.blocks
	f.Operations = nil
	return nil
}
func (t *Temporary) Close() error {
	if t == nil || t.file == nil {
		return nil
	}
	var e error
	if t.writeErr == nil {
		e = t.flushBatch()
	}
	if e == nil {
		e = t.buffer.Flush()
	}
	ce := t.file.Close()
	t.file = nil
	if e != nil {
		return e
	}
	return ce
}
