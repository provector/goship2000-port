// G2R storage version 2. Simulation and operation schemas are independent.
package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/zstd"
)

const blockTarget = 64 << 10
const maxRow = (2 << 20) + (64 << 10)
const descriptorSize = 48
const maxBlocks = MaxBytes / (2*descriptorSize + 1)
const footerSize = 44
const footerID = "G2R2DONE"

var blockPrefix = []byte("GS2FILM\x00\x04\x00\x00\x00")
var order = binary.LittleEndian
var invalidBlock = errors.New("invalid or incomplete G2R block structure")
var encoder, encoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)), zstd.WithEncoderConcurrency(1))
var decoder, decoderErr = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(2*MaxLaunchBytes), zstd.WithDecodeAllCapLimit(true))

type descriptor struct {
	Kind, Stored, Decoded, Count uint32
	Digest                       [32]byte
}
type frame struct {
	descriptor
	offset int64
	first  int
}
type recordedStart struct {
	Header Header
	Launch json.RawMessage
}
type manifest struct {
	Codec       string `json:",omitempty"`
	Header      Header
	End         End
	IndexOffset int64
	Blocks      int
	IndexDigest string
}
type blockSource struct {
	readRows    func([]byte, uint32) ([]Operation, error)
	managedFile os.FileInfo // published file selected by ID; a library rename preserves its inode
	limit       int64       // selected stored-byte cap for a private, sealed publication
	revision    string
	path        string
	memory      io.ReaderAt // used only by explicit in-memory imports
	pages       []indexPage
	frames      []frame // one authenticated directory page
	page        int
	decoded     int64
	count       int
	manifest    manifest
	cached      int
	rows        []Operation
}

func strictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return invalidBlock
	}
	return nil
}
func (d descriptor) valid(first bool) bool {
	limit := maxRow + 1
	if first {
		limit = MaxLaunchBytes + MaxHeaderBytes + 128
		if d.Kind != 1 || d.Count != 0 {
			return false
		}
	} else if d.Kind != 2 || d.Count == 0 || d.Count > d.Decoded/2 {
		return false
	}
	if !first && d.Count > 1 && d.Decoded > blockTarget {
		return false
	}
	return d.Decoded > 0 && int64(d.Decoded) <= int64(limit) && d.Stored > 0 && int64(d.Stored) <= int64(limit)+65536
}
func readDescriptor(r io.ReaderAt, at int64) (descriptor, error) {
	var d descriptor
	var b [descriptorSize]byte
	if _, e := r.ReadAt(b[:], at); e != nil {
		return d, e
	}
	err := binary.Read(bytes.NewReader(b[:]), order, &d)
	return d, err
}
func decodeFrame(r io.ReaderAt, f frame) ([]byte, error) {
	return decodeCodecFrame(r, f, "")
}
func decodeCodecFrame(r io.ReaderAt, f frame, codec string) ([]byte, error) {
	d, e := readDescriptor(r, f.offset)
	if e != nil {
		return nil, e
	}
	if d != f.descriptor {
		return nil, invalidBlock
	}
	stored := make([]byte, d.Stored)
	if _, e = r.ReadAt(stored, f.offset+descriptorSize); e != nil {
		return nil, e
	}
	if sha256.Sum256(stored) != d.Digest {
		return nil, fmt.Errorf("film frame digest differs")
	}
	if codec == protoS2 {
		return decodeS2(stored, int(d.Decoded))
	}
	if decoderErr != nil {
		return nil, decoderErr
	}
	raw, e := decoder.DecodeAll(stored, make([]byte, 0, d.Decoded))
	if e != nil {
		return nil, e
	}
	if len(raw) != int(d.Decoded) {
		return nil, invalidBlock
	}
	return raw, nil
}
func writeFrame(w io.Writer, kind, count uint32, b []byte) (descriptor, error) {
	var d descriptor
	if kind == 2 {
		return writeCompressedFrame(w, kind, count, len(b), s2.Encode(nil, b))
	}
	if encoderErr != nil {
		return d, encoderErr
	}
	return writeCompressedFrame(w, kind, count, len(b), encoder.EncodeAll(b, nil))
}
func writeCompressedFrame(w io.Writer, kind, count uint32, decoded int, data []byte) (descriptor, error) {
	d := descriptor{kind, uint32(len(data)), uint32(decoded), count, sha256.Sum256(data)}
	if !d.valid(kind == 1) {
		return d, invalidBlock
	}
	if e := binary.Write(w, order, d); e != nil {
		return d, e
	}
	_, e := w.Write(data)
	return d, e
}
func writeManifest(w io.Writer, m manifest) error {
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	header, _ := json.Marshal(m.Header)
	end, _ := json.Marshal(m.End)
	if len(header) > MaxHeaderBytes || len(end) > MaxEndBytes || len(b) > MaxHeaderBytes+MaxEndBytes+1024 {
		return invalidBlock
	}
	var tail [footerSize]byte
	copy(tail[:8], footerID)
	order.PutUint32(tail[8:12], uint32(len(b)))
	h := sha256.Sum256(b)
	copy(tail[12:], h[:])
	if _, e = w.Write(b); e != nil {
		return e
	}
	_, e = w.Write(tail[:])
	return e
}
func readManifest(r io.ReaderAt, size int64) (manifest, int64, string, error) {
	var m manifest
	var tail [footerSize]byte
	if e := checkPrefix(r); e != nil {
		return m, 0, "", e
	}
	if size < 12+footerSize || size > MaxBytes {
		return m, 0, "", invalidBlock
	}
	if _, e := r.ReadAt(tail[:], size-footerSize); e != nil {
		return m, 0, "", e
	}
	n := int64(order.Uint32(tail[8:12]))
	at := size - footerSize - n
	if string(tail[:8]) != footerID || n < 1 || n > MaxHeaderBytes+MaxEndBytes+1024 || at < 12 {
		return m, 0, "", invalidBlock
	}
	b := make([]byte, n)
	if _, e := r.ReadAt(b, at); e != nil {
		return m, 0, "", e
	}
	h := sha256.Sum256(b)
	if !bytes.Equal(h[:], tail[12:]) {
		return m, 0, "", invalidBlock
	}
	if e := strictJSON(b, &m); e != nil {
		return m, 0, "", e
	}
	hb, _ := json.Marshal(m.Header)
	eb, _ := json.Marshal(m.End)
	if len(hb) > MaxHeaderBytes || len(eb) > MaxEndBytes {
		return m, 0, "", invalidBlock
	}
	if m.Blocks < 1 || m.Blocks > maxBlocks || m.IndexOffset < 12 || m.IndexOffset > at-int64(m.Blocks)*descriptorSize || at-m.IndexOffset-int64(m.Blocks)*descriptorSize > MaxHeaderBytes+MaxEndBytes+1024+footerSize || len(m.IndexDigest) != 64 {
		return m, 0, "", invalidBlock
	}
	// Validate all shared policy/header/end invariants without fetching launch data.
	f := Film{Header: m.Header, Launch: json.RawMessage("null"), End: m.End}
	return m, at, fmt.Sprintf("%x", h), f.Validate()
}
func openBlocks(r io.ReaderAt, size int64, path string) (*Film, error) {
	f, _, e := openFormat(r, size, path, currentFormat())
	return f, e
}
func openFormat(r io.ReaderAt, size int64, path string, format ImportFormat) (*Film, *portableInfo, error) {
	m, _, revision, pub, e := readContainerFormat(r, size, format)
	if e != nil {
		return nil, nil, e
	}
	s := &blockSource{readRows: format.Rows, limit: MaxBytes, path: path, revision: revision, cached: -1, page: -1, manifest: m}
	if path == "" {
		s.memory = r
	}
	at := int64(len(blockPrefix))
	h := sha256.New()
	buf := make([]byte, indexPageSize*descriptorSize)
	for i := 0; i < m.Blocks; i += indexPageSize {
		part := buf[:min(indexPageSize, m.Blocks-i)*descriptorSize]
		if _, e = r.ReadAt(part, m.IndexOffset+int64(i)*descriptorSize); e != nil {
			return nil, nil, e
		}
		h.Write(part)
		s.pages = append(s.pages, indexPage{at, s.count, sha256.Sum256(part)})
		for j := 0; j < len(part); j += descriptorSize {
			d := parseDescriptor(part[j:])
			if !d.valid(i+j == 0) {
				return nil, nil, invalidBlock
			}
			at += descriptorSize + int64(d.Stored)
			s.count += int(d.Count)
			s.decoded += int64(d.Decoded)
			if at > m.IndexOffset {
				return nil, nil, invalidBlock
			}
		}
	}
	if at != m.IndexOffset || fmt.Sprintf("%x", h.Sum(nil)) != m.IndexDigest {
		return nil, nil, invalidBlock
	}
	if e = s.loadPage(r, 0); e != nil {
		return nil, nil, e
	}
	launch, e := decodeFrame(r, s.frames[0])
	if e != nil {
		return nil, nil, e
	}
	var start recordedStart
	if e = strictJSON(launch, &start); e != nil {
		return nil, nil, e
	}
	initial := start.Header
	initial.Title = m.Header.Title
	initial.Complete = m.Header.Complete
	if initial != m.Header || len(start.Launch) == 0 || len(start.Launch) > MaxLaunchBytes {
		return nil, nil, invalidBlock
	}
	visible := m.Header
	if pub != nil {
		visible.Title = pub.title
	}
	return &Film{Header: visible, Launch: start.Launch, End: m.End, blocks: s}, pub, nil
}
func (f *Film) BlockStorage() bool { return f != nil && f.blocks != nil }
func (f *Film) OperationCount() int {
	if f.blocks != nil {
		return f.blocks.count
	}
	return len(f.Operations)
}
func (f *Film) OperationAt(i int) (Operation, error) {
	if i < 0 || i >= f.OperationCount() {
		return Operation{}, invalidBlock
	}
	s := f.blocks
	if s == nil {
		return f.Operations[i], nil
	}
	page := sort.Search(len(s.pages), func(j int) bool { return s.pages[j].first > i }) - 1
	if s.cached >= 0 && page == s.page {
		part := s.frames[s.cached]
		if i >= part.first && i < part.first+int(part.Count) {
			return s.rows[i-part.first], nil
		}
	}
	var r io.ReaderAt = s.memory
	if r == nil {
		file, info, e := regularFilm(s.path)
		if s.managedFile != nil && (os.IsNotExist(e) || e == nil && !os.SameFile(info, s.managedFile)) {
			if file != nil {
				file.Close()
			}
			path, err := (Store{filepath.Dir(s.path)}).resolved(f.Header.ID, false)
			if err != nil {
				return Operation{}, err
			}
			file, info, e = regularFilm(path)
			if e == nil {
				if !os.SameFile(s.managedFile, info) {
					file.Close()
					return Operation{}, fmt.Errorf("selected recording was replaced during playback")
				}
				s.path = path
			}
		}
		if e != nil {
			return Operation{}, e
		}
		defer file.Close()
		r = file
	}
	if page != s.page {
		if e := s.loadPage(r, page); e != nil {
			return Operation{}, e
		}
	}
	j := sort.Search(len(s.frames), func(j int) bool { return s.frames[j].first+int(s.frames[j].Count) > i })
	raw, e := decodeCodecFrame(r, s.frames[j], s.manifest.Codec)
	if e != nil {
		return Operation{}, e
	}
	rows, e := s.readRows(raw, s.frames[j].Count)
	if e != nil {
		return Operation{}, e
	}
	s.rows = rows
	s.cached = j
	return s.rows[i-s.frames[j].first], nil
}

// Snapshot materializes a detached logical operation sequence for explicit offline consumers.
func (f *Film) Snapshot() (*Film, error) {
	clone := *f
	if f.blocks != nil && f.blocks.decoded > MaxSnapshotBytes || f.OperationCount() > MaxSnapshotBytes/256 {
		return nil, fmt.Errorf("explicit detached snapshot exceeds memory limit; use the streaming reader")
	}
	clone.blocks = nil
	clone.Launch = bytes.Clone(f.Launch)
	clone.End.Checkpoint = maps.Clone(f.End.Checkpoint)
	if f.End.Summary != nil {
		summary := *f.End.Summary
		clone.End.Summary = &summary
	}
	clone.Operations = make([]Operation, 0, f.OperationCount())
	for i := 0; i < f.OperationCount(); i++ {
		o, e := f.OperationAt(i)
		if e != nil {
			return nil, e
		}
		o.Payload = bytes.Clone(o.Payload)
		o.Checkpoint = maps.Clone(o.Checkpoint)
		o.Events = slices.Clone(o.Events)
		clone.Operations = append(clone.Operations, o)
	}
	if f.blocks == nil {
		return &clone, nil
	}
	// Caller may mutate the detached slices. Bind the normalized
	// hash to the storage identity only while those values still agree.
	logical, e := RecoveryContentIdentity(&clone)
	if e != nil {
		return nil, e
	}
	stored, e := RecoveryContentIdentity(f)
	if e != nil {
		return nil, e
	}
	clone.snapshotLogical, clone.snapshotStored = logical, stored
	return &clone, nil
}

// InspectPrefix checks complete consecutive frames. It never supplies an End.
func InspectPrefix(path string) (blocks, operations int, verifiedBytes int64, err error) {
	r, _, e := regularFilm(path)
	if e != nil {
		err = e
		return
	}
	defer r.Close()
	return inspectPrefix(r)
}
func inspectPrefix(r io.ReaderAt) (blocks, operations int, verifiedBytes int64, err error) {
	var prefix [12]byte
	_, err = r.ReadAt(prefix[:], 0)
	if err == nil && !bytes.Equal(prefix[:], privatePrefix) && !publishedPrefix(prefix[:]) {
		err = checkPrefix(r)
	}
	if err != nil {
		return
	}
	verifiedBytes = 12

	for blocks < maxBlocks {
		d, e := readDescriptor(r, verifiedBytes)
		if e != nil {
			err = e
			return
		}
		if !d.valid(blocks == 0) {
			err = invalidBlock
			return
		}
		codec := protoS2
		if blocks == 0 {
			codec = ""
		}
		raw, e := decodeCodecFrame(r, frame{d, verifiedBytes, operations}, codec)
		if e == nil {
			if blocks == 0 {
				var start recordedStart
				if strictJSON(raw, &start) != nil || !ValidID(start.Header.ID) || len(start.Launch) == 0 || len(start.Launch) > MaxLaunchBytes {
					e = invalidBlock
				}
			} else {
				_, e = decodeProtoRows(raw, d.Count)
			}
		}
		if e != nil {
			err = e
			return
		}
		verifiedBytes += descriptorSize + int64(d.Stored)

		blocks++
		operations += int(d.Count)
	}
	err = invalidBlock
	return
}
