package replay

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type limitedOutput struct {
	remaining int
	bytes.Buffer
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return w.Buffer.Write(p)
	}
	n, _ := w.Buffer.Write(p[:w.remaining])
	w.remaining = 0
	return n, io.ErrShortWrite
}
func TestCompressedFrameInterruptedWrites(t *testing.T) {
	var full bytes.Buffer
	if _, e := writeFrame(&full, 1, 0, []byte(`{"launch":"ordinary test"}`)); e != nil {
		t.Fatal(e)
	}
	for limit := 0; limit < full.Len(); limit++ {
		w := &limitedOutput{remaining: limit}
		_, e := writeFrame(w, 1, 0, []byte(`{"launch":"ordinary test"}`))
		if !errors.Is(e, io.ErrShortWrite) || !bytes.Equal(w.Bytes(), full.Bytes()[:limit]) {
			t.Fatal("frame write claimed completion or changed prefix", limit, e)
		}
	}
}
