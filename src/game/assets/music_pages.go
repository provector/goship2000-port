package assets

import (
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
)

// MusicPages contains immutable entry extents and the pages needed for immediate starts/loops.
type MusicPages struct {
	path   string
	item   *packedFile
	Frames int
	warm   map[int][]int16
}

const MusicPageFrames = musicChunkFrames

func (m *MusicPages) Warm(page int) []int16 { return m.warm[page] }
func (m *MusicPages) Open() (io.ReadSeekCloser, error) {
	f, _, err := regularPack(m.path, 256<<20)
	if err != nil {
		return nil, err
	}
	return f, nil
}
func (m *MusicPages) Read(file io.ReadSeekCloser, page int) ([]int16, error) {
	r, ok := file.(io.ReaderAt)
	if !ok || page < 0 || page >= len(m.item.parts) {
		return nil, fmt.Errorf("invalid music page %d", page)
	}
	b, err := m.item.chunk(r, page)
	if err != nil {
		return nil, err
	}
	samples := make([]int16, len(b)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return samples, nil
}
func (p *packedStore) music(o Output, frames, loop uint64) (*MusicPages, error) {
	item := p.catalog.Files[o.Path]
	if item == nil || item.Codec != "flac" || item.Size != o.Size || frames > 32<<19 || loop > frames || frames*2+44 != uint64(o.Size) {
		return nil, fmt.Errorf("packed music layout differs for %s", o.Path)
	}
	m := &MusicPages{path: filepath.Join(p.root, preparedPacks[item.Pack]), item: item, Frames: int(frames), warm: map[int][]int16{}}
	for _, first := range []int{0, int(loop) / MusicPageFrames} {
		for i := first; i < first+2 && i < len(item.parts); i++ {
			if m.warm[i] != nil {
				continue
			}
			page, err := m.Read(p.handles[item.Pack], i)
			if err != nil {
				return nil, err
			}
			m.warm[i] = page
		}
	}
	return m, nil
}
func (c *AudioClip) Frames() int {
	if c.Music != nil {
		return c.Music.Frames
	}
	return len(c.Samples)
}
