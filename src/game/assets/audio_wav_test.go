package assets

import (
	"testing"
)

func TestAudioUnsignedPCMAndMalformedRIFF(t *testing.T) {
	raw := append([]byte{82, 73, 70, 70, 39, 0, 0, 0, 87, 65, 86, 69, 102, 109, 116, 32, 16, 0, 0, 0, 1, 0, 1, 0, 64, 31, 0, 0, 64, 31, 0, 0, 1, 0, 8, 0, 100, 97, 116, 97, 3, 0, 0, 0}, 0, 128, 255)
	c, bits, e := decodeAudioWAV(raw)
	if e != nil || bits != 8 || len(c.Samples) != 3 || c.Samples[0] != -32768 || c.Samples[1] != 0 || c.Samples[2] != 32512 {
		t.Fatal(c, bits, e)
	}
	for _, bad := range [][]byte{raw[:10], raw[:len(raw)-1], append(append([]byte{}, raw...), 0)} {
		if _, _, e := decodeAudioWAV(bad); e == nil {
			t.Fatal("malformed WAV accepted")
		}
	}
}
