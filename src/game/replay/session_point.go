package replay

import (
	"bytes"
	"fmt"
	"io"
)

// SessionPoint owns the unflushed row block. Earlier frames in the same open
// recorder are immutable until this mission ends. No second file or disk format.
type SessionPoint struct {
	owner         *Temporary
	bytes         int64
	count, blocks int
	batch         uint32
	events        uint64
	pending       []byte
}

func (t *Temporary) Point() (*SessionPoint, error) {
	if t == nil || t.file == nil || t.failure != nil || t.writeErr != nil {
		return nil, fmt.Errorf("recording is unavailable")
	}
	if t.buffer.Buffered() != 0 {
		return nil, fmt.Errorf("recording frame is incomplete")
	}
	if err := t.checkPointSize(); err != nil {
		return nil, err
	}
	return &SessionPoint{t, t.bytes, t.count, t.blocks, t.batch, t.events, bytes.Clone(t.pending.Bytes())}, nil
}

// No history is read. Reject a damaged or incomplete spool before committing
// a bookmark or truncating its continuation.
func (t *Temporary) checkPointSize() error {
	st, err := t.file.Stat()
	if err != nil {
		return err
	}
	if st.Size() != t.bytes {
		return fmt.Errorf("recording extent changed: have %d bytes, expected %d", st.Size(), t.bytes)
	}
	return nil
}

func (p *SessionPoint) EventCount() uint64 { return p.events }

type PointReader struct {
	point           *SessionPoint
	offset          int64
	block, consumed int
	rows            []Operation
}

func (p *SessionPoint) Reader() *PointReader { return &PointReader{point: p, offset: 12} }
func (r *PointReader) Next() (Operation, error) {
	p := r.point
	t := p.owner
	if t.file == nil {
		return Operation{}, fmt.Errorf("recording closed")
	}
	for len(r.rows) == 0 {
		switch {
		case r.block < p.blocks:
			d, e := readDescriptor(t.file, r.offset)
			if e != nil {
				return Operation{}, e
			}
			if !d.valid(r.block == 0) || r.offset+descriptorSize+int64(d.Stored) > p.bytes {
				return Operation{}, invalidBlock
			}
			at := r.offset
			r.offset += descriptorSize + int64(d.Stored)
			r.block++
			if d.Kind == 1 {
				continue
			}
			raw, e := decodeCodecFrame(t.file, frame{d, at, r.consumed}, protoS2)
			if e != nil {
				return Operation{}, e
			}
			r.rows, e = decodeProtoRows(raw, d.Count)
			if e != nil {
				return Operation{}, e
			}
		case r.block == p.blocks:
			if r.offset != p.bytes {
				return Operation{}, invalidBlock
			}
			var e error
			r.rows, e = decodeProtoRows(p.pending, p.batch)
			if e != nil {
				return Operation{}, e
			}
			r.block++
		default:
			if r.consumed != p.count {
				return Operation{}, invalidBlock
			}
			return Operation{}, io.EOF
		}
	}
	o := r.rows[0]
	r.rows = r.rows[1:]
	r.consumed++
	return o, nil
}

// Restore commits the writer position after the caller validates the saved state.
// A failed truncate leaves the continuation intact; restore its write cursor.
func (p *SessionPoint) Restore(t *Temporary) error {
	if t != p.owner || t.file == nil || t.failure != nil || t.writeErr != nil || t.count < p.count || t.buffer.Buffered() != 0 {
		return fmt.Errorf("recording point is unavailable")
	}
	if err := t.checkPointSize(); err != nil {
		return err
	}
	before, e := t.file.Seek(0, io.SeekCurrent)
	if e != nil {
		return e
	}
	if before != t.bytes {
		return fmt.Errorf("recording write position changed")
	}
	if _, e = t.file.Seek(p.bytes, io.SeekStart); e != nil {
		return e
	}
	if e = t.file.Truncate(p.bytes); e != nil {
		if _, seek := t.file.Seek(before, io.SeekStart); seek != nil {
			t.writeErr = fmt.Errorf("restore failed: %w; restore write position: %v", e, seek)
			return &RecordingIOError{"restore-write-position", t.writeErr}
		}
		return e
	}
	t.bytes, t.blocks, t.count, t.batch, t.events = p.bytes, p.blocks, p.count, p.batch, p.events
	t.pending.Reset()
	t.pending.Write(p.pending)
	return nil
}
