package replay

import (
	"bytes"
	"testing"
)

// Exercise the recording envelope with a regular encoder-produced payload large
// enough to span multiple zstd blocks. Identity is the decoded mission content.
func TestMultiblockLaunchPayloadRoundTrip(t *testing.T) {
	body := bytes.Repeat([]byte("mission snapshot, aircraft and objectives\n"), 8192)
	var buf bytes.Buffer
	d, err := writeFrame(&buf, 1, 0, body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeFrame(bytes.NewReader(buf.Bytes()), frame{descriptor: d})
	if err != nil || !bytes.Equal(body, got) {
		t.Fatal("decoded launch differs", err)
	}
	if _, err = decodeFrame(bytes.NewReader(buf.Bytes()[:buf.Len()-1]), frame{descriptor: d}); err == nil {
		t.Fatal("incomplete frame accepted")
	}
}
