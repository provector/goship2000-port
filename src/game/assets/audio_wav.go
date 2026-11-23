package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Prepared WAVs are ordinary PCM. The digital bytes stay unsigned in the file;
// conversion here is only the playback view, never a rewrite of the raw bank.
func decodeAudioWAV(raw []byte) (*AudioClip, int, error) {
	if len(raw) < 12 || !bytes.Equal(raw[:4], []byte("RIFF")) || !bytes.Equal(raw[8:12], []byte("WAVE")) || uint64(binary.LittleEndian.Uint32(raw[4:8]))+8 != uint64(len(raw)) {
		return nil, 0, fmt.Errorf("invalid/truncated PCM RIFF/WAVE")
	}
	var payload []byte
	rate, bits, align := 0, 0, 0
	haveFmt, haveData := false, false
	for at := 12; at < len(raw); {
		if len(raw)-at < 8 {
			return nil, 0, fmt.Errorf("truncated WAV chunk")
		}
		size := uint64(binary.LittleEndian.Uint32(raw[at+4 : at+8]))
		end := uint64(at) + 8 + size
		if end > uint64(len(raw)) {
			return nil, 0, fmt.Errorf("WAV chunk outside file")
		}
		body := raw[at+8 : int(end)]
		switch string(raw[at : at+4]) {
		case "fmt ":
			if haveFmt || len(body) < 16 {
				return nil, 0, fmt.Errorf("duplicate/short WAV fmt")
			}
			haveFmt = true
			if le16(body) != 1 || le16(body[2:]) != 1 {
				return nil, 0, fmt.Errorf("prepared audio requires PCM mono")
			}
			rate = int(binary.LittleEndian.Uint32(body[4:8]))
			bits = le16(body[14:])
			align = le16(body[12:])
			if (bits != 8 && bits != 16) || align != bits/8 || uint64(binary.LittleEndian.Uint32(body[8:12])) != uint64(rate*align) {
				return nil, 0, fmt.Errorf("inconsistent WAV PCM layout")
			}
		case "data":
			if haveData {
				return nil, 0, fmt.Errorf("duplicate WAV data")
			}
			haveData = true
			payload = body
		}
		next := end + (size & 1)
		// Existing source-compatible U8 WAV writers need not append a final padding
		// byte. Interior chunks still require their RIFF word-alignment padding.
		if next > uint64(len(raw)) && end != uint64(len(raw)) {
			return nil, 0, fmt.Errorf("missing WAV padding")
		}
		at = int(next)
	}
	if !haveFmt || !haveData || align == 0 || len(payload) == 0 || len(payload)%align != 0 {
		return nil, 0, fmt.Errorf("missing/misaligned WAV PCM")
	}
	if !(bits == 16 && rate == AudioSampleRate || bits == 8 && rate == 8000) {
		return nil, 0, fmt.Errorf("unsupported prepared audio rate/bits %d/%d", rate, bits)
	}
	samples := make([]int16, len(payload)/align)
	for i := range samples {
		if bits == 16 {
			samples[i] = int16(binary.LittleEndian.Uint16(payload[2*i:]))
		} else {
			samples[i] = (int16(payload[i]) - 128) << 8
		}
	}
	return &AudioClip{Samples: samples, SampleRate: rate}, bits, nil
}
