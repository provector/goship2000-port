package assets

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

const musicChunkFrames = 32768

func encodePCMChunk(pcm []byte) ([]byte, error) {
	var buf bytes.Buffer
	n := len(pcm) / 2
	info := &meta.StreamInfo{BlockSizeMin: 4096, BlockSizeMax: 4096, SampleRate: AudioSampleRate, NChannels: 1, BitsPerSample: 16, NSamples: uint64(n), MD5sum: md5.Sum(pcm)}
	enc, err := flac.NewEncoder(&buf, info)
	if err != nil {
		return nil, err
	}
	for at := 0; at < n; at += 4096 {
		count := min(4096, n-at)
		samples := make([]int32, count)
		for i := range samples {
			samples[i] = int32(int16(binary.LittleEndian.Uint16(pcm[(at+i)*2:])))
		}
		f := frame.Frame{Header: frame.Header{HasFixedBlockSize: true, BlockSize: uint16(count), SampleRate: 0, Channels: frame.ChannelsMono, BitsPerSample: 16}, Subframes: []*frame.Subframe{{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, Samples: samples, NSamples: count}}}
		if err = enc.WriteFrame(&f); err != nil {
			return nil, err
		}
	}
	if err = enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodePCMChunk(b []byte, size int) ([]byte, error) {
	if size <= 0 || size > musicChunkFrames*2 || size%2 != 0 {
		return nil, fmt.Errorf("invalid FLAC PCM extent %d", size)
	}
	// Only our one STREAMINFO header is admitted. No arbitrary metadata allocation.
	if len(b) < 42 || string(b[:4]) != "fLaC" || !bytes.Equal(b[4:8], []byte{128, 0, 0, 34}) {
		return nil, fmt.Errorf("unexpected prepared FLAC header")
	}
	s, err := flac.New(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if s.Info.SampleRate != AudioSampleRate || s.Info.BitsPerSample != 16 || s.Info.NChannels != 1 || s.Info.NSamples != uint64(size/2) {
		return nil, fmt.Errorf("FLAC stream layout differs")
	}
	out := make([]byte, size)
	at := 0
	for at < size {
		f, err := s.ParseNext()
		if err != nil {
			return nil, err
		}
		if len(f.Subframes) != 1 || len(f.Subframes[0].Samples) == 0 || len(f.Subframes[0].Samples) > (size-at)/2 {
			return nil, fmt.Errorf("invalid FLAC frame length")
		}
		for _, v := range f.Subframes[0].Samples {
			if v < -32768 || v > 32767 {
				return nil, fmt.Errorf("FLAC sample overflow")
			}
			binary.LittleEndian.PutUint16(out[at:], uint16(v))
			at += 2
		}
	}
	if _, err = s.Next(); err != io.EOF {
		return nil, fmt.Errorf("trailing FLAC frame data")
	}
	if md5.Sum(out) != s.Info.MD5sum {
		return nil, fmt.Errorf("FLAC PCM digest mismatch")
	}
	return out, nil
}
