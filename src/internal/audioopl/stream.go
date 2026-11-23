package audioopl

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Stream isolates the pinned GPL synthesis component in its bundled process.
// Each transaction is bounded; its FM envelopes, phases and noise survive it.
// Use from an audio worker, never from a real-time device callback.
type Stream struct {
	command *exec.Cmd
	in      io.WriteCloser
	out     io.ReadCloser
	cleanup func()
	once    sync.Once
}

func OpenStream(rate int) (*Stream, error) {
	if rate < 8000 || rate > 192000 {
		return nil, fmt.Errorf("invalid OPL stream rate")
	}
	filename, cleanup, err := unpackHelper()
	if err != nil {
		return nil, err
	}
	c := exec.Command(filename, "--stream")
	configureHelperCommand(c)
	in, err := c.StdinPipe()
	if err != nil {
		cleanup()
		return nil, err
	}
	out, err := c.StdoutPipe()
	if err != nil {
		in.Close()
		cleanup()
		return nil, err
	}
	if err = c.Start(); err != nil {
		in.Close()
		out.Close()
		cleanup()
		return nil, fmt.Errorf("start bundled flight OPL helper: %w", err)
	}
	s := &Stream{command: c, in: in, out: out, cleanup: cleanup}
	header := append([]byte("OPL32\x00\x00\x00"), byte(rate), byte(rate>>8), byte(rate>>16), byte(rate>>24))
	if _, err = in.Write(header); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Render timestamps are relative to this transaction, in supplied order.
func (s *Stream) Render(frames int, writes []Write) ([]int32, error) {
	if frames <= 0 || frames > 8192 || len(writes) > 8192 {
		return nil, fmt.Errorf("invalid OPL stream transaction")
	}
	var input bytes.Buffer
	binary.Write(&input, binary.LittleEndian, uint32(frames))
	binary.Write(&input, binary.LittleEndian, uint32(len(writes)))
	var previous uint64
	for i, w := range writes {
		if w.Frame > uint64(frames) || w.Register > 255 || i > 0 && w.Frame < previous {
			return nil, fmt.Errorf("invalid OPL stream write")
		}
		previous = w.Frame
		binary.Write(&input, binary.LittleEndian, uint32(w.Frame))
		binary.Write(&input, binary.LittleEndian, w.Register)
		input.WriteByte(w.Value)
		input.WriteByte(0)
	}
	if _, err := s.in.Write(input.Bytes()); err != nil {
		return nil, err
	}
	pcm := make([]byte, frames*4)
	if _, err := io.ReadFull(s.out, pcm); err != nil {
		return nil, fmt.Errorf("flight OPL stream stopped: %w", err)
	}
	out := make([]int32, frames)
	for i := range out {
		out[i] = int32(binary.LittleEndian.Uint32(pcm[i*4:]))
	}
	return out, nil
}

func (s *Stream) Close() {
	s.once.Do(func() {
		s.in.Close()
		if s.command.Process != nil {
			_ = s.command.Process.Kill()
		}
		s.out.Close()
		_ = s.command.Wait()
		s.cleanup()
	})
}
