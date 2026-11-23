package audioopl

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"gunship2000-assets/internal/testfixture"
	"io"
	"os"
	"testing"
)

func TestBundledHelperIdentityAndSilence(t *testing.T) {
	compressed, err := nativePayload()
	if err != nil {
		t.Skip(err) // Unsupported preparation platform; no runtime/playback dependency.
	}
	z, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	id := Identity()
	h := sha256.Sum256(b)
	if id.ProtocolVersion != 1 || id.HelperSHA256 != hex.EncodeToString(h[:]) || id.License != "GPL-2.0-or-later" {
		t.Fatalf("component/helper identity mismatch: %+v", id)
	}
	pcm, err := Render(49716, 64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != 64 {
		t.Fatal(len(pcm))
	}
	for i, value := range pcm {
		if value != 0 {
			t.Fatalf("unkeyed helper sample%d=%d", i, value)
		}
	}
}

func TestTimelineBounds(t *testing.T) {
	cases := []struct {
		rate   int
		frames uint64
		writes []Write
	}{
		{1, 1, nil}, {49716, 49716 * 601, nil},
		{49716, 8, []Write{{Frame: 9}}},
		{49716, 8, []Write{{Register: 256}}},
		{49716, 8, []Write{{Frame: 5}, {Frame: 4}}},
	}
	for _, c := range cases {
		if _, err := Render(c.rate, c.frames, c.writes); err == nil {
			t.Fatalf("accepted malformed timeline %+v", c)
		}
	}
}

func TestPCMMatchesQualifiedOPLReference(t *testing.T) {
	if _, err := nativePayload(); err != nil {
		t.Skip(err)
	}
	b, err := os.ReadFile(testfixture.Path("opl-reference/accepted-reference.opl27"))
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		InputSHA256 string `json:"input_sha256"`
		Expected    string `json:"expected_pcm_sha256"`
	}
	meta, err := os.ReadFile(testfixture.Path("opl-reference/provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(meta, &proof); err != nil {
		t.Fatal(err)
	}
	inputHash := sha256.Sum256(b)
	if hex.EncodeToString(inputHash[:]) != proof.InputSHA256 {
		t.Fatal("reference timeline identity changed")
	}
	rate, count, frames := binary.LittleEndian.Uint32(b[8:]), binary.LittleEndian.Uint32(b[12:]), binary.LittleEndian.Uint64(b[16:])
	if len(b) != 28+12*int(count) {
		t.Fatal("reference framing")
	}
	writes := make([]Write, count)
	for i := range writes {
		at := 28 + 12*i
		writes[i] = Write{Frame: binary.LittleEndian.Uint64(b[at:]), Register: binary.LittleEndian.Uint16(b[at+8:]), Value: b[at+10]}
	}
	pcm, err := Render(int(rate), frames, writes)
	if err != nil {
		t.Fatal(err)
	}
	actual := make([]byte, len(pcm)*4)
	for i, v := range pcm {
		binary.LittleEndian.PutUint32(actual[i*4:], uint32(v))
	}
	h := sha256.Sum256(actual)
	if hex.EncodeToString(h[:]) != proof.Expected {
		t.Fatalf("PCM differs from immutable qualified reference: %x", h)
	}
}
