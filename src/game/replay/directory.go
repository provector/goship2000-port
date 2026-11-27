package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// A page anchor authenticates subsequent re-reads without retaining the whole directory.
const indexPageSize = 256

type indexPage struct {
	offset int64
	first  int
	digest [32]byte
}

func parseDescriptor(b []byte) descriptor {
	d := descriptor{Kind: order.Uint32(b[0:4]), Stored: order.Uint32(b[4:8]), Decoded: order.Uint32(b[8:12]), Count: order.Uint32(b[12:16])}
	copy(d.Digest[:], b[16:48])
	return d
}

func (s *blockSource) loadPage(r io.ReaderAt, page int) error {
	anchor := s.pages[page]
	n := min(indexPageSize, s.manifest.Blocks-page*indexPageSize)
	b := make([]byte, n*descriptorSize)
	if _, err := r.ReadAt(b, s.manifest.IndexOffset+int64(page)*indexPageSize*descriptorSize); err != nil {
		return err
	}
	if sha256.Sum256(b) != anchor.digest {
		return invalidBlock
	}
	at, first := anchor.offset, anchor.first
	s.frames = s.frames[:0]
	for j := 0; j < n; j++ {
		d := parseDescriptor(b[j*descriptorSize:])
		s.frames = append(s.frames, frame{d, at, first})
		at += descriptorSize + int64(d.Stored)
		first += int(d.Count)
	}
	s.page = page
	s.cached = -1
	return nil
}

func checkPrefix(r io.ReaderAt) error {
	var b [12]byte
	if _, err := r.ReadAt(b[:], 0); err != nil {
		return fmt.Errorf("film header: %w", err)
	}
	if !bytes.Equal(b[:8], magic[:]) {
		return fmt.Errorf("unrecognized film format")
	}
	if v := order.Uint16(b[8:10]); v != 4 {
		return fmt.Errorf("unsupported G2R storage version %d (expected current protobuf/S2)", v)
	}
	if !bytes.Equal(b[:], blockPrefix) {
		return invalidBlock
	}
	return nil
}

// Whole-document APIs are for detached edits and the historical cursor-repair tool.
// Normal recording, browsing, Export, playback and practice Resume stream from disk.
const MaxSnapshotBytes = 256 << 20

func Marshal(f *Film) ([]byte, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if f.OperationCount() > MaxSnapshotBytes/256 {
		return nil, fmt.Errorf("too many rows for a detached snapshot")
	}
	var out, idx, pending bytes.Buffer
	out.Write(blockPrefix)
	header := f.Header
	header.Title = "Recovery identity" // Mutable display metadata belongs in the manifest.
	start, err := json.Marshal(recordedStart{header, f.Launch})
	if err != nil {
		return nil, err
	}
	add := func(kind, count uint32, data []byte) error {
		d, err := writeFrame(&out, kind, count, data)
		if err != nil {
			return err
		}
		return binary.Write(&idx, order, d)
	}
	if err = add(1, 0, start); err != nil {
		return nil, err
	}
	decoded := int64(len(start))
	var count uint32
	for i := 0; i < f.OperationCount(); i++ {
		op, err := f.OperationAt(i)
		if err != nil {
			return nil, err
		}
		row, err := protoRow(op)
		if err != nil {
			return nil, err
		}
		decoded += int64(len(row) + 1)
		if decoded > MaxSnapshotBytes || len(row) > maxRow {
			return nil, fmt.Errorf("detached snapshot exceeds size limit")
		}
		if count > 0 && pending.Len()+len(row)+1 > blockTarget {
			if err = add(2, count, pending.Bytes()); err != nil {
				return nil, err
			}
			pending.Reset()
			count = 0
		}
		pending.Write(row)

		count++
	}
	if count > 0 {
		if err = add(2, count, pending.Bytes()); err != nil {
			return nil, err
		}
	}
	m := manifest{protoS2, f.Header, f.End, int64(out.Len()), idx.Len() / descriptorSize, BytesDigest(idx.Bytes())}
	out.Write(idx.Bytes())
	if err = writeManifest(&out, m); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Unmarshal(b []byte) (*Film, error) {
	f, err := openBlocks(bytes.NewReader(b), int64(len(b)), "")
	if err != nil {
		return nil, err
	}
	return f.Snapshot()
}
